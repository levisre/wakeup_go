package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/viper"
)

type targetMachine struct {
	Name string `mapstructure:"name"`
	Mac  string `mapstructure:"mac"`
	IP   string `mapstructure:"ip"`
	// Runtime-only fields (not from config, not serialized)
	mu       sync.Mutex
	cachedIP string
	lastSeen time.Time
}

// ipCacheTTL controls how long a discovered IP is considered valid.
// Configurable via "ip_cache_ttl" in config.json (Go duration string, e.g. "12h").
var ipCacheTTL = 12 * time.Hour

// resolvedIP returns the best-known IP for the target machine.
// Config IP always takes precedence; cached IP is used if still within TTL.
func (t *targetMachine) resolvedIP() string {
	if t.IP != "" {
		return t.IP
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cachedIP != "" && time.Since(t.lastSeen) < ipCacheTTL {
		return t.cachedIP
	}
	return ""
}

// updateCachedIP stores a discovered IP with a fresh timestamp.
// Thread-safe for use from goroutines.
func (t *targetMachine) updateCachedIP(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cachedIP = ip
	t.lastSeen = time.Now()
}

type pendingAction int

const (
	idle pendingAction = iota
	wake
	ping
	check
)

type userInfo struct {
	Username      string
	FirstName     string
	LastName      string
	ChatID        int64
	targetMachine []targetMachine
	pendingAction pendingAction
}

func PcPing(ip string) bool {
	var target_ip string
	// Ping the remote PC to check whether it's online
	if host, _, err := net.SplitHostPort(ip); err == nil {
		target_ip = host
	} else {
		target_ip = ip
	}
	_, err := exec.Command("ping", target_ip, "-c", "1").Output()
	if err != nil {
		return false
	}
	return true
}

func PCWakeUpCheck(ip string) bool {
	// Infinity loop and ping to check whether PC goes online or not
	for {
		if !PcPing(ip) {
			log.Println("Machine is not waken up yet!")
			// Sleep for 5 second between each ping
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
	return true
}

// ARPWakeUpCheck polls the ARP table for a target MAC address after sending
// a WOL packet. It sends broadcast pings on the given interfaces to stimulate
// ARP table population (since Linux doesn't add entries from Gratuitous ARP
// for unknown hosts). Returns the discovered IP when the MAC appears, or
// empty string on timeout.
func ARPWakeUpCheck(mac net.HardwareAddr, ifaces []InterfaceInfo, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Send broadcast pings to stimulate ARP responses on each subnet
		for _, iface := range ifaces {
			bcast := BroadcastAddr(iface.Network)
			if bcast != nil {
				// ping -b -c 1 -W 1 <broadcast> — best effort, ignore errors
				_ = exec.Command("ping", "-b", "-c", "1", "-W", "1", bcast.String()).Run()
			}
		}

		// Check ARP table for target MAC
		ip, err := LookupIPByMAC(mac)
		if err != nil {
			log.Printf("ARP lookup error: %v", err)
		}
		if ip != nil {
			return ip.String()
		}

		log.Printf("Waiting for %s to appear in ARP table...", mac)
		time.Sleep(5 * time.Second)
	}
	return ""
}

func main() {
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	var botToken, RemotePcIP, RemotePCMacAddr, wolPasswd, apiEndpoint string
	var allowedChatIDs []int64
	var bot *tgbotapi.BotAPI
	var targets []targetMachine
	users := make(map[int64]*userInfo)

	// Read configuration file
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	if viper.InConfig("bot_token") {
		botToken = viper.GetString("bot_token")
	}
	if viper.InConfig("chat_ids") {
		// New format: array of allowed chat IDs
		for _, id := range viper.GetIntSlice("chat_ids") {
			allowedChatIDs = append(allowedChatIDs, int64(id))
		}
	} else if viper.InConfig("chat_id") {
		// Legacy format: single chat ID
		allowedChatIDs = append(allowedChatIDs, viper.GetInt64("chat_id"))
	}
	log.Printf("Allowed %d chat ID(s)", len(allowedChatIDs))
	if viper.InConfig("remote_ip") {
		RemotePcIP = viper.GetString("remote_ip")
	}
	if viper.InConfig("remote_mac") {
		RemotePCMacAddr = viper.GetString("remote_mac")
	}

	if viper.InConfig("wol_passwd") {
		wolPasswd = viper.GetString("wol_passwd")
	}
	if viper.InConfig("targets") {
		if err := viper.UnmarshalKey("targets", &targets); err != nil {
			log.Println("Failed to read targets from config:", err)
		}
	}
	if viper.InConfig("ip_cache_ttl") {
		ipCacheTTL = viper.GetDuration("ip_cache_ttl")
		log.Printf("IP cache TTL set to %s", ipCacheTTL)
	} else {
		log.Printf("IP cache TTL using default: %s", ipCacheTTL)
	}
	// Use this for additional API endpoint, e.g: proxy
	if viper.InConfig("api_endpoint") {
		apiEndpoint = viper.GetString("api_endpoint")
	}
	// If no API endpoint is provided, use the default Telegram API endpoint
	if apiEndpoint == "" {
		apiEndpoint = tgbotapi.APIEndpoint
	}
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(botToken, apiEndpoint)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Helper to check if a chat ID is allowed
	isAllowedChatID := func(id int64) bool {
		for _, allowed := range allowedChatIDs {
			if allowed == id {
				return true
			}
		}
		return false
	}

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil { // If we got a message
			var msg tgbotapi.MessageConfig
			var txtMessage string
			// Check Chat ID, only accept messages from allowed IDs
			if !isAllowedChatID(update.Message.From.ID) {
				// Log unauthorized access
				log.Printf("Unauthorized access from %s (ID: %d)", update.Message.From.UserName, update.Message.From.ID)
				continue
			}

			// Get or create per-user state
			chatID := update.Message.Chat.ID
			user, exists := users[chatID]
			if !exists {
				user = &userInfo{
					targetMachine: targets,
					pendingAction: idle,
				}
				users[chatID] = user
			}
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
			user.Username = update.Message.From.UserName
			user.FirstName = update.Message.From.FirstName
			user.LastName = update.Message.From.LastName
			user.ChatID = chatID

			{
				// Handle non-command messages when there's a pending action
				// Here with the help of vibe coding (Gemini + Codex)
				if !update.Message.IsCommand() {
					if len(user.targetMachine) > 0 && user.pendingAction != idle {
						var selected *targetMachine
						for i := range user.targetMachine {
							if user.targetMachine[i].Name == update.Message.Text {
								selected = &user.targetMachine[i]
								break
							}
						}
						if selected != nil {
							switch user.pendingAction {
							case wake:
								txtMessage = sendWake(bot, user.ChatID, selected, wolPasswd)
							case check:
								ip := selected.resolvedIP()
								if ip == "" {
									// Try on-demand ARP lookup as fallback
									if mac, err := net.ParseMAC(selected.Mac); err == nil {
										if found, _ := LookupIPByMAC(mac); found != nil {
											ip = found.String()
											selected.updateCachedIP(ip)
											log.Printf("ARP lookup discovered %s for %s", ip, selected.Name)
										}
									}
								}
								if ip != "" {
									if PcPing(ip) {
										txtMessage = fmt.Sprintf("Machine %s is **online**! (IP: %s)", selected.Name, ip)
									} else {
										txtMessage = fmt.Sprintf("Machine %s is **offline**!", selected.Name)
									}
								} else {
									txtMessage = fmt.Sprintf("Machine %s: no IP known (try /wake first)", selected.Name)
								}
							default:
								txtMessage = "Invalid Command"
							}
							msg = tgbotapi.NewMessage(user.ChatID, txtMessage)
							msg.ParseMode = "markdown"
							msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
							_, err := bot.Send(msg)
							if err != nil {
								return
							}
							user.pendingAction = idle // Reset pending action
							continue
						}
					}
					txtMessage = "Invalid Command"
				} else {
					switch update.Message.Command() {
					case "wake":
						if len(user.targetMachine) > 0 {
							// Ask user to choose a target machine via reply keyboard
							var rows [][]tgbotapi.KeyboardButton
							for _, t := range user.targetMachine {
								if t.Name == "" {
									continue
								}
								btn := tgbotapi.NewKeyboardButton(t.Name)
								rows = append(rows, tgbotapi.NewKeyboardButtonRow(btn))
							}
							msg = tgbotapi.NewMessage(user.ChatID, "Choose a machine to wake:")
							rk := tgbotapi.NewReplyKeyboard(rows...)
							rk.ResizeKeyboard = true
							msg.ReplyMarkup = rk
							_, err := bot.Send(msg)
							if err != nil {
								return
							}
							user.pendingAction = wake
							continue
						}
						// Fallback to single configured machine
						legacyTarget := &targetMachine{IP: RemotePcIP, Mac: RemotePCMacAddr, Name: "legacy"}
						txtMessage = sendWake(bot, user.ChatID, legacyTarget, wolPasswd)

					case "check":
						if len(user.targetMachine) > 0 {
							// Ask user to choose a target machine via reply keyboard
							var rows [][]tgbotapi.KeyboardButton
							for _, t := range user.targetMachine {
								if t.Name == "" {
									continue
								}
								btn := tgbotapi.NewKeyboardButton(t.Name)
								rows = append(rows, tgbotapi.NewKeyboardButtonRow(btn))
							}
							msg = tgbotapi.NewMessage(user.ChatID, "Choose a machine to ping:")
							rk := tgbotapi.NewReplyKeyboard(rows...)
							rk.ResizeKeyboard = true
							msg.ReplyMarkup = rk
							_, err := bot.Send(msg)
							if err != nil {
								return
							}
							user.pendingAction = check
							continue
						}
						// Fallback to single configured machine
						wakeStatus := PcPing(RemotePcIP)
						if wakeStatus {
							txtMessage = "Machine is **online**!"
						} else {
							txtMessage = "Machine is **offline**!"
						}
					case "list":
						// List all configured target machines
						if len(user.targetMachine) > 0 {
							names := make([]string, 0, len(user.targetMachine))
							for _, t := range user.targetMachine {
								if t.Name != "" {
									names = append(names, t.Name)
								}
							}
							if len(names) == 0 {
								txtMessage = "No machines configured"
							} else {
								txtMessage = "Available machines:\n- " + strings.Join(names, "\n- ")
							}
						} else if RemotePcIP != "" || RemotePCMacAddr != "" {
							txtMessage = "Single machine configured"
						} else {
							txtMessage = "No machines configured"
						}
					case "hello":
						txtMessage = fmt.Sprintf("Hello! %s %s\nIm your servant!", user.FirstName, user.LastName)
					default:
						txtMessage = "I don't understand"
					}
				}
				msg = tgbotapi.NewMessage(user.ChatID, txtMessage)
				msg.ParseMode = "markdown"
				_, err := bot.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}
}

func sendWake(bot *tgbotapi.BotAPI, chatID int64, target *targetMachine, wolPasswd string) string {
	// Convert target MAC Addr
	targetMac, err := net.ParseMAC(target.Mac)
	if err != nil {
		log.Printf("Invalid MAC address: %s", target.Mac)
		return "Failed to send Packet"
	}

	resolvedIP := target.resolvedIP()
	wolSent := false

	if resolvedIP != "" {
		// IP is known: send both UDP and raw for maximum reliability.
		// UDP may silently fail (ARP timeout for powered-off targets),
		// so we always follow up with a raw Ethernet broadcast.

		// 1. Try UDP (best effort, no root required)
		if err := wakeUDP(resolvedIP, targetMac, []byte(wolPasswd)); err != nil {
			log.Printf("UDP WOL failed: %v", err)
		} else {
			log.Printf("UDP WOL packet sent to %s", resolvedIP)
			wolSent = true
		}

		// 2. Always try raw packet as well (requires CAP_NET_RAW)
		ifaces, ifErr := GetActiveInterfaces()
		if ifErr != nil {
			log.Println("Failed to detect network interfaces:", ifErr)
		} else {
			matchedIface, ifErr := FindInterfaceForIP(resolvedIP, ifaces)
			if ifErr != nil {
				log.Println("Failed to resolve interface for IP:", ifErr)
			} else if matchedIface == nil {
				log.Printf("No local interface found matching subnet for %s", resolvedIP)
			} else {
				log.Printf("Sending raw WOL via %s for target %s", matchedIface.Name, resolvedIP)
				if err := wakeRaw(matchedIface.Name, targetMac, []byte(wolPasswd)); err != nil {
					log.Printf("Raw WOL failed on %s: %v", matchedIface.Name, err)
				} else {
					wolSent = true
				}
			}
		}
	} else {
		// No IP configured or cache expired: broadcast raw WOL on all active interfaces
		log.Println("No IP configured for target, broadcasting raw WOL on all active interfaces...")
		ifaces, ifErr := GetActiveInterfaces()
		if ifErr != nil {
			log.Println("Failed to detect network interfaces:", ifErr)
			return "Failed to send Packet"
		}
		for _, iface := range ifaces {
			if err := wakeRaw(iface.Name, targetMac, []byte(wolPasswd)); err != nil {
				log.Printf("Failed to send raw WOL on %s: %v", iface.Name, err)
				continue
			}
			log.Printf("Sent raw WOL packet via %s to %s", iface.Name, targetMac)
			wolSent = true
		}
	}

	if wolSent {
		if resolvedIP != "" {
			// IP is known: monitor via ping
			go func() {
				wakeStatus := PCWakeUpCheck(resolvedIP)
				if wakeStatus {
					txtMessage := "Machine is online!"
					sttMsg := tgbotapi.NewMessage(chatID, txtMessage)
					_, err := bot.Send(sttMsg)
					if err != nil {
						log.Println(err)
					}
				}
			}()
		} else {
			// No IP: monitor via ARP table to discover the DHCP-assigned IP
			go func() {
				ifaces, ifErr := GetActiveInterfaces()
				if ifErr != nil {
					log.Println("Cannot monitor ARP table:", ifErr)
					return
				}
				discoveredIP := ARPWakeUpCheck(targetMac, ifaces, 5*time.Minute)
				if discoveredIP != "" {
					target.updateCachedIP(discoveredIP)
					log.Printf("Discovered IP %s for %s, cached for %s", discoveredIP, target.Mac, ipCacheTTL)
					txtMessage := fmt.Sprintf("Machine is online! IP: %s", discoveredIP)
					sttMsg := tgbotapi.NewMessage(chatID, txtMessage)
					_, err := bot.Send(sttMsg)
					if err != nil {
						log.Println(err)
					}
				} else {
					log.Printf("Timeout waiting for %s in ARP table", targetMac)
					sttMsg := tgbotapi.NewMessage(chatID, "Timeout: machine did not appear on network")
					_, _ = bot.Send(sttMsg)
				}
			}()
		}
		return "Wake packet sent to machine!"
	}
	return "Failed to send Packet"
}
