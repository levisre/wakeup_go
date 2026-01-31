package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/viper"
)

type targetMachine struct {
	Name string `mapstructure:"name"`
	Mac  string `mapstructure:"mac"`
	IP   string `mapstructure:"ip"`
}

type pendingAction int

const (
	idle pendingAction = iota
	wake
	ping
	check
)

func PcPing(ip string) bool {
	// Ping the remote PC to check whether it's online
	target_ip := strings.Split(ip, ":")[0]
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

func main() {
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	var botToken, RemotePcIP, RemotePCMacAddr, inetInterface, wolPasswd, apiEndpoint string
	var MyChatId int64
	var bot *tgbotapi.BotAPI
	var targets []targetMachine
	var pendingAction pendingAction

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	if viper.InConfig("bot_token") {
		botToken = viper.GetString("bot_token")
	}
	if viper.InConfig("chat_id") {
		MyChatId = viper.GetInt64("chat_id")
	}
	if viper.InConfig("remote_ip") {
		RemotePcIP = viper.GetString("remote_ip")
	}
	if viper.InConfig("remote_mac") {
		RemotePCMacAddr = viper.GetString("remote_mac")
	}
	if viper.InConfig("inet_interface") {
		inetInterface = viper.GetString("inet_interface")
	}
	if viper.InConfig("wol_passwd") {
		wolPasswd = viper.GetString("wol_passwd")
	}
	if viper.InConfig("targets") {
		if err := viper.UnmarshalKey("targets", &targets); err != nil {
			log.Println("Failed to read targets from config:", err)
		}
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

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil { // If we got a message
			var msg tgbotapi.MessageConfig
			var txtMessage string
			// Check Chat ID, only accept message from my ID
			if update.Message.From.ID != MyChatId {
				// Report abuse username to me
				txtMessage = fmt.Sprintf("Unauthorized access from %s", update.Message.From.UserName)
				//	msg = tgbotapi.NewMessage(MyChatId, txtMessage)
				// OK, now if the message was sent from me
			} else {
				log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
				//msg.ReplyToMessageID = update.Message.MessageID
				// Handle non-command messages when there's a pending action
				// Here with the help of vibe coding (Gemini + Codex)
				if !update.Message.IsCommand() {
					if len(targets) > 0 && pendingAction != idle {
						var selected *targetMachine
						for i := range targets {
							if targets[i].Name == update.Message.Text {
								selected = &targets[i]
								break
							}
						}
						if selected != nil {
							switch pendingAction {
							case wake:
								txtMessage = sendWake(bot, MyChatId, selected.IP, selected.Mac, inetInterface, wolPasswd)
							case check:
								if PcPing(selected.IP) {
									txtMessage = fmt.Sprintf("Machine %s is **online**!", selected.Name)
								} else {
									txtMessage = fmt.Sprintf("Machine %s is **offline**!", selected.Name)
								}
							default:
								txtMessage = "Invalid Command"
							}
							msg = tgbotapi.NewMessage(MyChatId, txtMessage)
							msg.ParseMode = "markdown"
							msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
							_, err := bot.Send(msg)
							if err != nil {
								return
							}
							pendingAction = idle // Reset pending action
							continue
						}
					}
					txtMessage = "Invalid Command"
				} else {
					switch update.Message.Command() {
					case "wake":
						if len(targets) > 0 {
							// Ask user to choose a target machine via reply keyboard
							var rows [][]tgbotapi.KeyboardButton
							for _, t := range targets {
								if t.Name == "" {
									continue
								}
								btn := tgbotapi.NewKeyboardButton(t.Name)
								rows = append(rows, tgbotapi.NewKeyboardButtonRow(btn))
							}
							msg = tgbotapi.NewMessage(MyChatId, "Choose a machine to wake:")
							rk := tgbotapi.NewReplyKeyboard(rows...)
							rk.ResizeKeyboard = true
							msg.ReplyMarkup = rk
							_, err := bot.Send(msg)
							if err != nil {
								return
							}
							pendingAction = wake
							continue
						}
						// Fallback to single configured machine
						txtMessage = sendWake(bot, MyChatId, RemotePcIP, RemotePCMacAddr, inetInterface, wolPasswd)

					case "check":
						if len(targets) > 0 {
							// Ask user to choose a target machine via reply keyboard
							var rows [][]tgbotapi.KeyboardButton
							for _, t := range targets {
								if t.Name == "" {
									continue
								}
								btn := tgbotapi.NewKeyboardButton(t.Name)
								rows = append(rows, tgbotapi.NewKeyboardButtonRow(btn))
							}
							msg = tgbotapi.NewMessage(MyChatId, "Choose a machine to ping:")
							rk := tgbotapi.NewReplyKeyboard(rows...)
							rk.ResizeKeyboard = true
							msg.ReplyMarkup = rk
							_, err := bot.Send(msg)
							if err != nil {
								return
							}
							pendingAction = check
							continue
						}
						wakeStatus := PcPing(RemotePcIP)
						if wakeStatus {
							txtMessage = "Machine is **online**!"
						} else {
							txtMessage = "Machine is **offline**!"
						}
					case "list":
						// List all configured target machines
						if len(targets) > 0 {
							names := make([]string, 0, len(targets))
							for _, t := range targets {
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
						txtMessage = fmt.Sprintf("Hello! %s %s\nIm your servant!", update.Message.From.FirstName, update.Message.From.LastName)
					default:
						txtMessage = "I don't understand"
					}
				}
				msg = tgbotapi.NewMessage(MyChatId, txtMessage)
				msg.ParseMode = "markdown"
				_, err := bot.Send(msg)
				if err != nil {
					return
				}
			}
		}
	}
}

func sendWake(bot *tgbotapi.BotAPI, chatID int64, ip string, mac string, inetInterface string, wolPasswd string) string {
	// Convert target MAC Addr
	targetMac, err := net.ParseMAC(mac)
	wolSent := false
	if err == nil {
		// Send wakeup signal
		if err := wakeUDP(ip, targetMac, []byte(wolPasswd)); err != nil {
			log.Println("There was error, trying again with raw packet...")
			log.Println(err)
			// Try again with raw Packet
			if err := wakeRaw(inetInterface, targetMac, []byte(wolPasswd)); err != nil {
				log.Println(err)
				return "Failed to send Packet"
			}
			wolSent = true
		} else {
			wolSent = true
		}
		if wolSent {
			// Run a goroutine to continuously check whether machine is up
			go func() {
				wakeStatus := PCWakeUpCheck(ip) // Hang here
				if wakeStatus {                 // Only executed when the machine is up
					txtMessage := "Machine is online!"
					sttMsg := tgbotapi.NewMessage(chatID, txtMessage)
					_, err := bot.Send(sttMsg)
					if err != nil {
						log.Println(err)
					}
					return
				}
			}()
			return "Wake packet sent to machine!"
		}
	}
	return "Failed to send Packet"
}
