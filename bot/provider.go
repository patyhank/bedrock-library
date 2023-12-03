package bot

import (
	"errors"
	"fmt"
	"git.patyhank.net/falloutBot/bedrocklib/item"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"reflect"
	"time"
)

var ErrConnected = errors.New("already connected")

type ClientConfig struct {
	Address string
	Token   *oauth2.Token
}

type Client struct {
	connected bool
	Conn      *minecraft.Conn
	events    *Events
	world     *World
	Logger    *log.Logger
	Screen    *ScreenManager
	Entity    *EntityManager
	Self      *Player
}

func init() {
	world.RegisterItem(item.EmptyMap{})
}

func NewClient() *Client {
	logger := log.New()
	customFormatter := new(log.TextFormatter)

	customFormatter.TimestampFormat = "15:04:05"
	customFormatter.FullTimestamp = true
	customFormatter.ForceColors = true
	customFormatter.ForceColors = true
	log.SetFormatter(customFormatter)
	logger.SetFormatter(customFormatter)
	client := &Client{
		events: &Events{
			generic:  []GenericHandler{},
			handlers: map[uint32][]any{},
			tickers:  []TickHandler{},
		},
		Logger: logger,
	}

	return client
}

func (c *Client) ConnectTo(config ClientConfig) error {
	if c.connected {
		return ErrConnected
	}
	c.connected = true
	tkn := auth.RefreshTokenSource(config.Token)
	serverConn, err := minecraft.Dialer{
		TokenSource: tkn,
		ClientData: login.ClientData{
			DeviceModel:   "WTF OS 1.0",
			DeviceOS:      protocol.DeviceAndroid,
			GameVersion:   "1.20.40",
			LanguageCode:  "zh_TW",
			ServerAddress: config.Address,
		},
	}.Dial("raknet", config.Address)
	if err != nil {
		return err
	}
	c.Conn = serverConn
	return nil
}

func (c *Client) HandleGame() error {
	err := c.Conn.DoSpawnTimeout(time.Second * 10)
	if err != nil {
		return err
	}
	c.Conn.WritePacket(&packet.Respawn{
		State: 2,
	})

	for {
		pk, err := c.Conn.ReadPacket()
		if err != nil {
			return err
		}
		id := pk.ID()
		handlers := c.events.handlers[id]
		if len(handlers) > 0 {
			for _, handler := range handlers {
				res := reflect.ValueOf(handler).FieldByName("F").Call([]reflect.Value{reflect.ValueOf(c), reflect.ValueOf(pk)})
				err := res[0].Interface()
				if err != nil {
					break
				}
			}
		}
	}
}

func (c *Client) FlyTo() {

}

func (c *Client) OpenContainer(pos protocol.BlockPos) error {
	wID := c.Screen.openedWindowID.Load()
	stack, _ := c.Screen.inv.Item(0)

	c.Conn.WritePacket(&packet.InventoryTransaction{
		TransactionData: &protocol.UseItemTransactionData{
			ActionType:      protocol.UseItemActionClickBlock,
			BlockPosition:   protocol.BlockPos{int32(pos.X()), int32(pos.Y()), int32(pos.Z())},
			BlockFace:       int32(0),
			ClickedPosition: mgl32.Vec3{0.5, 0.5, 0.5},
			HeldItem:        InstanceFromItem(stack),
			HotBarSlot:      0,
		},
	})
	ticker := time.NewTicker(time.Millisecond * 500)
	for i := 0; i < 20; i++ {
		<-ticker.C
		if c.Screen.openedWindowID.Load() != wID {
			return nil
		}
	}
	return fmt.Errorf("container opened timeout %v", pos)
}
func (c *Client) OpenSystemContainer(command string) error {
	wID := c.Screen.openedWindowID.Load()
	err := c.SendCommand(command)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(time.Millisecond * 500)
	for i := 0; i < 20; i++ {
		<-ticker.C
		if c.Screen.openedWindowID.Load() != wID {
			return nil
		}
	}
	return fmt.Errorf("container opened timeout, command: %v", command)
}
func (c *Client) SendText(command string) error {
	return c.Conn.WritePacket(&packet.Text{
		TextType: packet.TextTypeChat,
		Message:  command,
	})
}
func (c *Client) SendCommand(command string) error {
	return c.Conn.WritePacket(&packet.CommandRequest{
		CommandLine: command,
		CommandOrigin: protocol.CommandOrigin{
			Origin: protocol.CommandOriginPlayer,
			UUID:   uuid.Nil,
		},
		Internal: false,
	})
}
func (c *Client) OpenInventory() {
	c.Conn.WritePacket(&packet.Interact{
		ActionType:            packet.InteractActionOpenInventory,
		TargetEntityRuntimeID: c.Conn.GameData().EntityRuntimeID,
		Position:              mgl32.Vec3{0, 0, 0},
	})
}
