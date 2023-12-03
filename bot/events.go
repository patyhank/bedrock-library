package bot

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sandertv/gophertunnel/minecraft/text"
	_ "unsafe"
)

type EventsListener struct {
	dimensionData    []cube.Range
	currentDimension int
	air              uint32
}

func (e EventsListener) Attach(c *Client) {
	e.dimensionData = append(e.dimensionData, cube.Range{-64, 319}, cube.Range{0, 127}, cube.Range{0, 255})
	e.air, _ = chunk.StateToRuntimeID("minecraft:air", nil)

	c.Screen = NewManager(c)
	c.Entity = NewEntityManager()

	AddListener(c, PacketHandler[*packet.Text]{
		Priority: 64,
		F: func(client *Client, p *packet.Text) error {
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
			c.world = world.New()
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
			setChunk(c.world, world.ChunkPos(p.Position), ch, map[cube.Pos]world.Block{})
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
}

func (c *Client) World() *world.World {
	return c.world
}

//go:linkname setChunk github.com/df-mc/dragonfly/server/world.(*World).setChunk
func setChunk(world *world.World, pos world.ChunkPos, c *chunk.Chunk, e map[cube.Pos]world.Block)
