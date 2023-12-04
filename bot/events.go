package bot

import (
	"context"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	_ "github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/goxiaoy/go-eventbus"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/text"
	log "github.com/sirupsen/logrus"
	"time"
	_ "unsafe"
)

type EventsListener struct {
	dimensionData    []cube.Range
	currentDimension int
	air              uint32
}

func (e EventsListener) Attach(c *Client) {
	err := c.Conn.DoSpawnTimeout(time.Second * 10)
	if err != nil {
		panic(err)
	}
	c.Conn.WritePacket(&packet.Respawn{
		State: 2,
	})

	e.dimensionData = append(e.dimensionData, cube.Range{-64, 319}, cube.Range{0, 127}, cube.Range{0, 255})
	e.air, _ = chunk.StateToRuntimeID("minecraft:air", nil)
	c.EventBus = eventbus.New()
	c.Screen = NewManager(c)
	c.Entity = NewEntityManager()
	c.Self = &Player{
		Positioner: &Positioner{
			Position: c.Conn.GameData().PlayerPosition,
			Pitch:    c.Conn.GameData().Pitch,
			Yaw:      c.Conn.GameData().Yaw,
			HeadYaw:  c.Conn.GameData().Yaw,
		},
		Username:        c.Conn.IdentityData().DisplayName,
		EntityRuntimeID: c.Conn.GameData().EntityRuntimeID,
		PlatformChatID:  c.Conn.ClientData().PlatformOnlineID,
	}

	AddListener(c, PacketHandler[*packet.Text]{
		Priority: 64,
		F: func(client *Client, p *packet.Text) error {
			err := eventbus.Publish[*ChatEvent](c.EventBus)(context.Background(), &ChatEvent{Message: text.Clean(p.Message), FormattedMessage: p.Message})
			if err != nil {
				return nil
			}

			c.Logger.Info(text.ANSI(p.Message))
			return nil
		},
	})

	AddListener(c, PacketHandler[*packet.DimensionData]{
		Priority: 64,
		F: func(client *Client, p *packet.DimensionData) error {
			for _, definition := range p.Definitions {
				e.dimensionData = append(e.dimensionData, cube.Range{int(definition.Range[0]), int(definition.Range[1])})
			}
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.ChangeDimension]{
		Priority: 64,
		F: func(client *Client, p *packet.ChangeDimension) error {
			e.currentDimension = int(p.Dimension)
			c.world = NewWorld(e.dimensionData[e.currentDimension])
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.LevelChunk]{
		Priority: 64,
		F: func(client *Client, p *packet.LevelChunk) error {
			ch, err := chunk.NetworkDecode(e.air, p.RawPayload, int(p.SubChunkCount), e.dimensionData[e.currentDimension])
			if err != nil {
				return err
			}

			if c.world == nil {
				c.world = NewWorld(e.dimensionData[e.currentDimension])
			}

			c.world.setChunk(world.ChunkPos(p.Position), ch)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.AddActor]{
		Priority: 64,
		F: func(client *Client, p *packet.AddActor) error {
			c.Entity.AddEntity(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.AddPlayer]{
		Priority: 64,
		F: func(client *Client, p *packet.AddPlayer) error {
			c.Entity.AddPlayer(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.AddItemActor]{
		Priority: 64,
		F: func(client *Client, p *packet.AddItemActor) error {
			c.Entity.AddItems(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.RemoveActor]{
		Priority: 64,
		F: func(client *Client, p *packet.RemoveActor) error {
			c.Entity.RemoveEntity(p.EntityUniqueID)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.MoveActorAbsolute]{
		Priority: 64,
		F: func(client *Client, p *packet.MoveActorAbsolute) error {
			c.Entity.MoveEntity(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.MoveActorDelta]{
		Priority: 64,
		F: func(client *Client, p *packet.MoveActorDelta) error {
			c.Entity.MoveEntityDel(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.MovePlayer]{
		Priority: 64,
		F: func(client *Client, p *packet.MovePlayer) error {
			if p.EntityRuntimeID == c.Conn.GameData().EntityRuntimeID {
				log.Info("Teleported", p.Position)
				if p.Mode == packet.MoveModeTeleport || p.Mode == packet.MoveModeReset {
					c.Conn.WritePacket(&packet.PlayerAction{
						EntityRuntimeID: c.Self.EntityRuntimeID,
						ActionType:      protocol.PlayerActionHandledTeleport,
					})
				}
				c.Self.Position = p.Position

				c.Conn.WritePacket(p)
				return nil
			}
			c.Entity.MovePlayer(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.UpdateBlock]{
		F: func(client *Client, p *packet.UpdateBlock) error {
			client.world.setBlock(blockPosFromProtocol(p.Position), p.NewBlockRuntimeID)
			if p.NewBlockRuntimeID == air {
				go func() {
					err := eventbus.Publish[*BrokeBlockEvent](c.EventBus)(context.Background(), &BrokeBlockEvent{
						Position: p.Position,
					})
					if err != nil {
						return
					}
				}()
			}
			return nil
		},
	})
}

func (c *Client) World() *World {
	return c.world
}

//go:linkname setChunk github.com/df-mc/dragonfly/server/world.(*World).setChunk
func setChunk(world *world.World, pos world.ChunkPos, c *chunk.Chunk, e map[cube.Pos]world.Block)

// Events Sections

type BrokeBlockEvent struct {
	Position protocol.BlockPos
}

type ChatEvent struct {
	Message          string
	FormattedMessage string
}
