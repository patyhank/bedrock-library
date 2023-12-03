package screen

import (
	"fmt"
	"github.com/df-mc/dragonfly/server/item"
)

type Generic struct {
	Width, Height int          // Width and Height of the grid. Used to determine the offset of the non-content slots.
	Slots         []item.Stack // Will be initialized in InitGenericContainer
}

func InitGenericContainer(size, width, height int) *Generic {
	return &Generic{
		Width:  width,
		Height: height,
		Slots:  make([]item.Stack, size+36),
	}
}

func (g *Generic) OnClose() error {
	return nil
}

/* Slot data */

func (g *Generic) getSize() int                  { return g.Width * g.Height }
func (g *Generic) GetContentSlots() []item.Stack { return g.Slots[:g.getSize()] }
func (g *Generic) GetInventorySlots() []item.Stack {
	return g.Slots[g.getSize() : g.getSize()+35]
}
func (g *Generic) GetHotbarSlots() []item.Stack {
	return g.Slots[len(g.Slots)-9:]
}

/* Getter & Setter */

func (g *Generic) GetSlot(i int) item.Stack { return g.Slots[i] }
func (g *Generic) SetSlot(i int, s item.Stack) error {
	if i < 0 || i >= len(g.Slots) {
		return fmt.Errorf("slot index %d out of bounds. maximum index is %d", i, len(g.Slots)-1)
	}
	g.Slots[i] = s
	//g.Slots[i].Index = pk.Short(i)
	return nil
}

func (g *Generic) Count() int {
	return len(g.Slots)
}

func (g *Generic) Slot() []item.Stack {
	return g.Slots
}
