package main

import (
	"github.com/patyhank/bedrock-library/bot"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"log"
)

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

	bot.AddListener(client, bot.PacketHandler[*packet.Text]{ // Listen any text packet
		Priority: 1024,
		F: func(client *bot.Client, p *packet.Text) error {
			log.Println(p.Message)
			return nil
		},
	})

	err = client.HandleGame()
	if err != nil {
		panic(err)
	}
}
