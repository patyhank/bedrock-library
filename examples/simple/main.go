package main

import (
	"github.com/patyhank/bedrock-library/bot"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/text"
	"log"
	"regexp"
	"slices"
	"strings"
	"time"
)

var recvPMRegex = regexp.MustCompile("^\\[(\\w+) -> 您]\\s([\\s\\S]+)")
var recvTeleportRegex = regexp.MustCompile("^\\[系統] (\\w+) 想要傳送到 你 的位置。")
var recvTeleportHereRegex = regexp.MustCompile("^\\[系統] (\\w+) 想要你傳送到 該玩家 的位置。")

var owners = []string{"patyhank"}
var ads = []string{"test", "test1"}

func main() {
	token, err := auth.RequestLiveToken()
	if err != nil {
		panic(err)
	}

	client := bot.NewClient()
	err = client.ConnectTo(bot.ClientConfig{
		Address: "bedrock.mcfallout.net:19132",
		Token:   token,
	})
	if err != nil {
		panic(err)
	}

	(&bot.EventsListener{}).Attach(client)

	bot.AddListener(client, bot.PacketHandler[*packet.Text]{ // Listen any text packet
		Priority: 1024,
		F: func(client *bot.Client, p *packet.Text) error {
			log.Println(text.ANSI(p.Message)) // Print the colored message to the console
			cleanString := text.Clean(p.Message)
			if result := recvPMRegex.FindStringSubmatch(cleanString); result != nil {
				if slices.Contains(owners, result[1]) {
					args := strings.Split(result[2], " ")
					switch args[0] {
					case "cmd":
						client.SendCommand(strings.Join(args[1:], " "))
					case "chat":
						client.SendText(strings.Join(args[1:], " "))
					}
				}
			}

			if result := recvTeleportRegex.FindStringSubmatch(cleanString); result != nil {
				if slices.Contains(owners, result[1]) {
					client.SendCommand("/tpaccept " + result[1])
				}
			}

			if result := recvTeleportHereRegex.FindStringSubmatch(cleanString); result != nil {
				if slices.Contains(owners, result[1]) {
					client.SendCommand("/tpaccept")
				}
			}

			return nil
		},
	})

	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		if len(ads) == 0 {
			return
		}
		for {
			for _, ad := range ads {
				<-ticker.C
				if strings.HasPrefix(ad, "/") { // If the ad starts with a slash, it's a command
					client.SendCommand(ad) // Send a command packet every 10 minutes
				} else {
					client.SendText(ad) // Send a text packet every 10 minutes
				}
			}
		}
	}()

	err = client.HandleGame()
	if err != nil {
		panic(err)
	}
}
