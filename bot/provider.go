package bot

import (
	"context"
	"errors"
	"fmt"
	"git.patyhank.net/falloutBot/bedrocklib/item"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/goxiaoy/go-eventbus"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"reflect"
	"sync"
	"time"
)

var ErrConnected = errors.New("already connected")

type ClientConfig struct {
	Address string
	Token   *oauth2.Token
}

type Client struct {
	connected    bool
	config       ClientConfig
	Conn         *minecraft.Conn
	events       *Events
	world        *World
	Logger       *log.Logger
	Screen       *ScreenManager
	Entity       *EntityManager
	Self         *Player
	flyLock      sync.Mutex
	breakLock    sync.Mutex
	teleportChan chan any
	EventBus     *eventbus.EventBus
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
		Logger:       logger,
		flyLock:      sync.Mutex{},
		breakLock:    sync.Mutex{},
		teleportChan: make(chan any, 255),
	}

	return client
}

func (c *Client) ConnectTo(config ClientConfig) error {
	//if c.connected {
	//	return ErrConnected
	//}
	//c.connected = true
	c.config = config
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
	for {
		pk, err := c.Conn.ReadPacket()
		if err != nil {
			c.connected = false
			return err
		}
		id := pk.ID()
		handlers := c.events.handlers[id]
		if len(handlers) > 0 {
			go func() {
				for _, handler := range handlers {
					res := reflect.ValueOf(handler).FieldByName("F").Call([]reflect.Value{reflect.ValueOf(c), reflect.ValueOf(pk)})
					err := res[0].Interface()
					if err != nil {
						break
					}
				}
			}()
		}
	}
}

func (c *Client) Reconnect() error {
	err := c.ConnectTo(c.config)
	if err != nil {
		panic(err)
	}

	fmt.Println("Connected")
	return c.HandleGame()
}

func (c *Client) OpenContainer(pos protocol.BlockPos) error {
	wID := c.Screen.OpenedWindowID.Load()
	stack, _ := c.Screen.Inv.Item(0)

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
		if c.Screen.OpenedWindowID.Load() != wID {
			return nil
		}
	}
	return fmt.Errorf("container opened timeout %v", pos)
}
func (c *Client) OpenSystemContainer(command string) error {
	wID := c.Screen.OpenedWindowID.Load()
	err := c.SendCommand(command)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(time.Millisecond * 500)
	for i := 0; i < 20; i++ {
		<-ticker.C
		if c.Screen.OpenedWindowID.Load() != wID {
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

func (c *Client) PlaceBlock(pos cube.Pos) {
	hotbarSlot := int(c.Screen.HeldSlot.Load())
	stack, _ := c.Screen.Inv.Item(hotbarSlot)

	c.Conn.WritePacket(&packet.InventoryTransaction{
		TransactionData: &protocol.UseItemTransactionData{
			ActionType:      protocol.UseItemActionClickBlock,
			BlockPosition:   protocol.BlockPos{int32(pos.X()), int32(pos.Y()), int32(pos.Z())},
			BlockFace:       int32(0),
			ClickedPosition: mgl32.Vec3{0.5, 0.5, 0.5},
			HeldItem:        InstanceFromItem(stack),
			HotBarSlot:      int32(hotbarSlot),
		},
	})
}
func (c *Client) BreakBlock(pos cube.Pos) {
	c.breakLock.Lock()
	defer c.breakLock.Unlock()
	bPos := protocol.BlockPos{int32(pos[0]), int32(pos[1]), int32(pos[2])}

	c.Conn.WritePacket(&packet.PlayerAction{
		EntityRuntimeID: c.Self.EntityRuntimeID,
		ActionType:      protocol.PlayerActionStartBreak,
		BlockPosition:   bPos,
		BlockFace:       0,
	})

	broke := make(chan any, 255)
	disposable, _ := eventbus.Subscribe[*BrokeBlockEvent](c.EventBus)(func(ctx context.Context, event *BrokeBlockEvent) error {
		if event.Position == bPos {
			if c.World().Block(pos) != airB {
				return nil
			}
			go func() {
				broke <- 0
			}()
		}
		return nil
	})

	j, _ := c.Screen.Inv.Item(int(c.Screen.HeldSlot.Load()))
	c.Conn.WritePacket(&packet.InventoryTransaction{
		TransactionData: &protocol.UseItemTransactionData{
			ActionType:      protocol.UseItemActionBreakBlock,
			BlockPosition:   bPos,
			BlockFace:       int32(0),
			ClickedPosition: mgl32.Vec3{0.5, 0.5, 0.5},
			HeldItem:        InstanceFromItem(j),
			HotBarSlot:      int32(c.Screen.HeldSlot.Load()),
		},
	})

	breakTime := block.BreakDuration(c.World().Block(pos), j)
	if !c.Self.OnGround {
		breakTime *= 5
	}
	duration := breakTime / 10

	t := time.NewTicker(duration)
	for i := 0; i < 5; i++ {
		c.Conn.WritePacket(&packet.PlayerAction{
			EntityRuntimeID: c.Self.EntityRuntimeID,
			ActionType:      protocol.PlayerActionContinueDestroyBlock,
			BlockPosition:   bPos,
			ResultPosition:  bPos,
			BlockFace:       0,
		})
		select {
		case <-broke:
			break
		case <-t.C:
			continue
		}
	}
	t.Stop()
	disposable.Dispose()
}
