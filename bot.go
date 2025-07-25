package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/viper"
)

func PcPing(RemotePcIP string) bool {
	_, err := exec.Command("ping", RemotePcIP, "-c", "1").Output()
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
				if !update.Message.IsCommand() {
					txtMessage = "Invalid Command"
				} else {
					switch update.Message.Command() {
					case "wake":
						// Convert target MAC Addr
						targetMac, err := net.ParseMAC(RemotePCMacAddr)
						wolSent := false
						if err == nil {
							// Send wakeup signal
							if err := wakeUDP(RemotePcIP, targetMac, []byte(wolPasswd)); err != nil {
								log.Println("There was error, trying again with raw packet...")
								log.Println(err)
								// Try again with raw Packet
								if err := wakeRaw(inetInterface, targetMac, []byte(wolPasswd)); err != nil {
									log.Println(err)
									txtMessage = "Failed to send Packet"
								} else {
									wolSent = true
								}

							} else {
								wolSent = true
							}
							if wolSent {
								txtMessage = "Wake packet sent to machine!"
								// Run a goroutine to continuously check whether machine is up
								go func() {
									wakeStatus := PCWakeUpCheck(RemotePcIP) // Hang here
									if wakeStatus {                         // Only executed when the machine is up
										txtMessage := "Machine is online!"
										sttMsg := tgbotapi.NewMessage(MyChatId, txtMessage)
										_, err := bot.Send(sttMsg)
										if err != nil {
											log.Println(err)
										}
										return
									}
								}()
							}
						}

					case "check":
						wakeStatus := PcPing(RemotePcIP)
						if wakeStatus {
							txtMessage = "Machine is **online**!"
						} else {
							txtMessage = "Machine is **offline**!"
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
