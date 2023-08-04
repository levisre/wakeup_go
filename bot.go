package main

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/viper"
	"log"
	"net"
	"os/exec"
	"time"
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
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	botToken := viper.GetString("bot_token")
	MyChatId := viper.GetInt64("chat_id")
	RemotePcIP := viper.GetString("remote_ip")
	RemotePCMacAddr := viper.GetString("remote_mac")
	inetInterface := viper.GetString("inet_interface")
	bot, err := tgbotapi.NewBotAPI(botToken)
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
							if err := wakeUDP(RemotePcIP, targetMac, nil); err != nil {
								log.Println("There was error, trying again with raw packet...")
								log.Println(err)
								// Try again with raw Packet
								if err := wakeRaw(inetInterface, targetMac, nil); err != nil {
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
						txtMessage = fmt.Sprintf("Hello! `%s`\nIm your servant!", update.Message.From.UserName)
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
