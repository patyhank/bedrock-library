package bot

import (
	"git.patyhank.net/falloutBot/bedrocklib/internal/nbtconv"
	"github.com/df-mc/atomic"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/inventory"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type ScreenManager struct {
	c *Client

	inv, offHand, enderChest, ui *inventory.Inventory
	armour                       *inventory.Armour

	inTransaction, containerOpened atomic.Bool
	openedWindowID                 atomic.Uint32
	openedContainerID              atomic.Uint32
	openedWindow                   atomic.Value[*inventory.Inventory]
	handler                        *itemStackRequestHandler
	openedPos                      atomic.Value[cube.Pos]
}

func NewManager(client *Client) *ScreenManager {
	m := &ScreenManager{
		c:                 client,
		offHand:           inventory.New(1, nil),
		armour:            inventory.NewArmour(nil),
		inv:               inventory.New(36, nil),
		enderChest:        inventory.New(27, nil),
		ui:                inventory.New(53, nil),
		inTransaction:     atomic.Bool{},
		containerOpened:   atomic.Bool{},
		openedWindowID:    atomic.Uint32{},
		openedContainerID: atomic.Uint32{},
		openedWindow:      atomic.Value[*inventory.Inventory]{},
		openedPos:         atomic.Value[cube.Pos]{},
		handler:           &itemStackRequestHandler{changes: map[byte]map[byte]changeInfo{}, responseChanges: map[int32]map[*inventory.Inventory]map[byte]responseChange{}},
	}

	AddListener(client, PacketHandler[*packet.ContainerOpen]{
		F: func(client *Client, p *packet.ContainerOpen) error {
			if p.ContainerPosition != (protocol.BlockPos{}) {
				m.openedPos.Store(cube.Pos{int((p.ContainerPosition)[0]), int((p.ContainerPosition)[1]), int((p.ContainerPosition)[2])})
			}
			m.containerOpened.Store(true)
			m.openedWindowID.Store(uint32(p.WindowID))
			m.openedContainerID.Store(uint32(p.ContainerType))
			inv, b := m.invByID(int32(p.ContainerType))
			if b {
				m.openedWindow.Store(inv)
			}
			return nil
		},
	})
	AddListener(client, PacketHandler[*packet.InventoryContent]{
		F: func(client *Client, p *packet.InventoryContent) error {
			if p.WindowID == protocol.WindowIDInventory {
				for i, instance := range p.Content {
					m.inv.SetItem(i, StackToItem(instance.Stack))
				}
				return nil
			}
			if p.WindowID == protocol.WindowIDOffHand {
				for i, instance := range p.Content {
					m.offHand.SetItem(i, StackToItem(instance.Stack))
				}
				return nil
			}
			if p.WindowID == protocol.WindowIDArmour {
				helmet := StackToItem(p.Content[0].Stack)
				chestplate := StackToItem(p.Content[1].Stack)
				leggings := StackToItem(p.Content[2].Stack)
				boots := StackToItem(p.Content[3].Stack)
				m.armour.Set(helmet, chestplate, leggings, boots)
				return nil
			}
			if p.WindowID == protocol.WindowIDUI {
				for i, instance := range p.Content {
					m.ui.SetItem(i, StackToItem(instance.Stack))
				}
				return nil
			}
			win := m.openedWindow.Load()
			for i, instance := range p.Content {
				win.SetItem(i, StackToItem(instance.Stack))
			}

			return nil
		},
	})
	AddListener(client, PacketHandler[*packet.InventorySlot]{
		F: func(client *Client, p *packet.InventorySlot) error {
			if p.WindowID == protocol.WindowIDInventory {
				m.inv.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
				return nil
			}
			if p.WindowID == protocol.WindowIDOffHand {
				m.offHand.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
				return nil
			}
			if p.WindowID == protocol.WindowIDArmour {
				switch p.Slot {
				case 0:
					m.armour.SetHelmet(StackToItem(p.NewItem.Stack))
				case 1:
					m.armour.SetChestplate(StackToItem(p.NewItem.Stack))
				case 2:
					m.armour.SetLeggings(StackToItem(p.NewItem.Stack))
				case 3:
					m.armour.SetBoots(StackToItem(p.NewItem.Stack))
				}
				return nil
			}
			if p.WindowID == protocol.WindowIDUI {
				m.ui.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
				return nil
			}
			win := m.openedWindow.Load()
			win.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
			return nil
		},
	})

	return m
}

func (m *ScreenManager) SendContainerClick(request *packet.ItemStackRequest) error {
	err := m.handler.Handle(request, m)
	if err != nil {
		return err
	}
	return m.c.Conn.WritePacket(request)
}
func (m *ScreenManager) CloseCurrentWindow() {
	m.c.Conn.WritePacket(&packet.ContainerClose{
		WindowID:   byte(m.openedWindowID.Load()),
		ServerSide: false,
	})
	m.openedWindowID.Store(0)
}

// invByID attempts to return an inventory by the ID passed. If found, the inventory is returned and the bool
// returned is true.
func (m *ScreenManager) invByID(id int32) (*inventory.Inventory, bool) {
	switch id {
	case protocol.ContainerCraftingInput, protocol.ContainerCreatedOutput, protocol.ContainerCursor:
		// UI inventory.
		return m.ui, true
	case protocol.ContainerHotBar, protocol.ContainerInventory, protocol.ContainerCombinedHotBarAndInventory:
		// Hotbar 'inventory', rest of inventory, inventory when container is opened.
		return m.inv, true
	case protocol.ContainerOffhand:
		return m.offHand, true
	case protocol.ContainerArmor:
		// Armour inventory.
		return m.armour.Inventory(), true
	case protocol.ContainerLevelEntity:
		if m.containerOpened.Load() {
			b := m.c.World().Block(m.openedPos.Load())
			if _, chest := b.(block.Chest); chest {
				return m.openedWindow.Load(), true
			} else if _, enderChest := b.(block.EnderChest); enderChest {
				return m.openedWindow.Load(), true
			}
		}
	case protocol.ContainerBarrel:
		if m.containerOpened.Load() {
			if _, barrel := m.c.World().Block(m.openedPos.Load()).(block.Barrel); barrel {
				return m.openedWindow.Load(), true
			}
		}
	case protocol.ContainerBeaconPayment:
		if m.containerOpened.Load() {
			if _, beacon := m.c.World().Block(m.openedPos.Load()).(block.Beacon); beacon {
				return m.ui, true
			}
		}
	case protocol.ContainerAnvilInput, protocol.ContainerAnvilMaterial:
		if m.containerOpened.Load() {
			if _, anvil := m.c.World().Block(m.openedPos.Load()).(block.Anvil); anvil {
				return m.ui, true
			}
		}
	case protocol.ContainerSmithingTableInput, protocol.ContainerSmithingTableMaterial:
		if m.containerOpened.Load() {
			if _, smithing := m.c.World().Block(m.openedPos.Load()).(block.SmithingTable); smithing {
				return m.ui, true
			}
		}
	case protocol.ContainerLoomInput, protocol.ContainerLoomDye, protocol.ContainerLoomMaterial:
		if m.containerOpened.Load() {
			if _, loom := m.c.World().Block(m.openedPos.Load()).(block.Loom); loom {
				return m.ui, true
			}
		}
	case protocol.ContainerStonecutterInput:
		if m.containerOpened.Load() {
			if _, ok := m.c.World().Block(m.openedPos.Load()).(block.Stonecutter); ok {
				return m.ui, true
			}
		}
	case protocol.ContainerGrindstoneInput, protocol.ContainerGrindstoneAdditional:
		if m.containerOpened.Load() {
			if _, ok := m.c.World().Block(m.openedPos.Load()).(block.Grindstone); ok {
				return m.ui, true
			}
		}
	case protocol.ContainerEnchantingInput, protocol.ContainerEnchantingMaterial:
		if m.containerOpened.Load() {
			if _, enchanting := m.c.World().Block(m.openedPos.Load()).(block.EnchantingTable); enchanting {
				return m.ui, true
			}
		}
	case protocol.ContainerFurnaceIngredient, protocol.ContainerFurnaceFuel, protocol.ContainerFurnaceResult,
		protocol.ContainerBlastFurnaceIngredient, protocol.ContainerSmokerIngredient:
		if m.containerOpened.Load() {
			if _, ok := m.c.World().Block(m.openedPos.Load()).(smelter); ok {
				return m.openedWindow.Load(), true
			}
		}
	}
	return nil, false
}

// smelter is an interface representing a block used to smelt items.
type smelter interface {
	// ResetExperience resets the collected experience of the smelter, and returns the amount of experience that was reset.
	ResetExperience() int
}

// StackFromItem converts an item.Stack to its network ItemStack representation.
func StackFromItem(it item.Stack) protocol.ItemStack {
	if it.Empty() {
		return protocol.ItemStack{}
	}

	var blockRuntimeID uint32
	if b, ok := it.Item().(world.Block); ok {
		blockRuntimeID = world.BlockRuntimeID(b)
	}

	rid, meta, _ := world.ItemRuntimeID(it.Item())

	return protocol.ItemStack{
		ItemType: protocol.ItemType{
			NetworkID:     rid,
			MetadataValue: uint32(meta),
		},
		HasNetworkID:   true,
		Count:          uint16(it.Count()),
		BlockRuntimeID: int32(blockRuntimeID),
		NBTData:        nbtconv.WriteItem(it, false),
	}
}

// StackToItem converts a network ItemStack representation back to an item.Stack.
func StackToItem(it protocol.ItemStack) item.Stack {
	t, ok := world.ItemByRuntimeID(it.NetworkID, int16(it.MetadataValue))
	if !ok {
		t = block.Air{}
	}
	if it.BlockRuntimeID > 0 {
		// It shouldn't matter if it (for whatever reason) wasn't able to get the block runtime ID,
		// since on the next line, we assert that the block is an item. If it didn't succeed, it'll
		// return air anyway.
		b, _ := world.BlockByRuntimeID(uint32(it.BlockRuntimeID))
		if t, ok = b.(world.Item); !ok {
			t = block.Air{}
		}
	}
	//noinspection SpellCheckingInspection
	if nbter, ok := t.(world.NBTer); ok && len(it.NBTData) != 0 {
		t = nbter.DecodeNBT(it.NBTData).(world.Item)
	}
	s := item.NewStack(t, int(it.Count))
	return nbtconv.Item(it.NBTData, &s)
}

// InstanceFromItem converts an item.Stack to its network ItemInstance representation.
func InstanceFromItem(it item.Stack) protocol.ItemInstance {
	return protocol.ItemInstance{
		StackNetworkID: item_id(it),
		Stack:          StackFromItem(it),
	}
}
