package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	_ "github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/goxiaoy/go-eventbus"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
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
	e.currentDimension = int(c.Conn.GameData().Dimension)

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

			c.Logger.Info(text.ANSI(p.Message))
			go func() {
				err := eventbus.Publish[*ChatEvent](c.EventBus)(context.Background(), &ChatEvent{Message: text.Clean(p.Message), FormattedMessage: p.Message})
				if err != nil {
					return
				}
			}()
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

			b := bytes.NewBuffer(p.RawPayload)

			for i := 0; i < int(p.SubChunkCount); i++ {
				index := uint8(i)
				decodeSubChunk(b, chunk.New(air, e.dimensionData[e.currentDimension]), &index, chunk.NetworkEncoding)
			}
			n := (e.dimensionData[e.currentDimension].Height() >> 4) + 1
			for i := 0; i < int(n); i++ {
				decodePalettedStorage(b, chunk.NetworkEncoding, chunk.BiomePaletteEncoding)
			}

			_, err = b.ReadByte()
			if err != nil {
				log.Warn(err)
			}
			var bNBT map[string]any
			dec := nbt.NewDecoderWithEncoding(b, nbt.NetworkLittleEndian)
			bEnts := map[cube.Pos]map[string]any{}
			for {
				err := dec.Decode(&bNBT)
				if err != nil {
					break
				}
				pos := cube.Pos{int(bNBT["x"].(int32)), int(bNBT["y"].(int32)), int(bNBT["z"].(int32))}
				bEnts[pos] = bNBT
			}

			c.world.setChunk(world.ChunkPos(p.Position), ch, bEnts)
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
	AddListener(c, PacketHandler[*packet.ModalFormRequest]{
		Priority: 64,
		F: func(client *Client, p *packet.ModalFormRequest) error {
			var data Form
			json.Unmarshal(p.FormData, &data)
			data.ID = p.FormID
			c.CurrentForm = &data
			go eventbus.Publish[*Form](c.EventBus)(context.Background(), c.CurrentForm)
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
				c.Self.Yaw = p.Yaw
				c.Self.Pitch = p.Pitch
				c.Self.HeadYaw = p.HeadYaw
				c.Self.OnGround = p.OnGround

				c.Conn.WritePacket(p)
				return nil
			}
			c.Entity.MovePlayer(p)
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.UpdateBlock]{
		F: func(client *Client, p *packet.UpdateBlock) error {
			if p.Layer != 0 {
				return nil
			}
			//if p.Flags&packet.BlockUpdateNetwork != p.Flags {
			//	return nil
			//}
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
	AddListener(c, PacketHandler[*packet.UpdateSubChunkBlocks]{
		F: func(client *Client, p *packet.UpdateSubChunkBlocks) error {
			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.PlayerList]{
		F: func(client *Client, p *packet.PlayerList) error {
			if p.ActionType == packet.PlayerListActionAdd {
				for _, entry := range p.Entries {
					if c.Conn.IdentityData().XUID == entry.XUID {
						c.PlayerName = entry.Username
					}
				}
			}

			return nil
		},
	})
	AddListener(c, PacketHandler[*packet.BlockActorData]{
		F: func(client *Client, p *packet.BlockActorData) error {
			client.World().SetBlockEntity(cube.Pos{int(p.Position[0]), int(p.Position[1]), int(p.Position[2])}, p.NBTData)
			return nil
		},
	})
}

func (c *Client) World() *World {
	return c.world
}

//go:linkname decodePalettedStorage github.com/df-mc/dragonfly/server/world/chunk.decodePalettedStorage
func decodePalettedStorage(buf *bytes.Buffer, e chunk.Encoding, pe any) (*chunk.PalettedStorage, error)

//go:linkname decodeBiomes github.com/df-mc/dragonfly/server/world/chunk.decodeBiomes
func decodeBiomes(buf *bytes.Buffer, c *chunk.Chunk, e chunk.Encoding)

//go:linkname decodeSubChunk github.com/df-mc/dragonfly/server/world/chunk.decodeSubChunk
func decodeSubChunk(buf *bytes.Buffer, c *chunk.Chunk, index *byte, e chunk.Encoding) (*chunk.SubChunk, error)

// Events Sections

type BrokeBlockEvent struct {
	Position protocol.BlockPos
}

type ChatEvent struct {
	Message          string
	FormattedMessage string
}

// Form  FormSections
type Form struct {
	ID        uint32   `json:"_"`
	Buttons   []Button `json:"buttons,omitempty"`
	ButtonYes string   `json:"button1"`
	ButtonNo  string   `json:"button2"`
	Content   string   `json:"content,omitempty"`
	Title     string   `json:"title,omitempty"`
	Type      string   `json:"type,omitempty"`
}

type Button struct {
	Image map[string]string `json:"image,omitempty"`
	Text  string            `json:"text,omitempty"`
}
