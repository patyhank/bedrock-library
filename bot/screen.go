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

	Inv, OffHand, EnderChest, UI *inventory.Inventory
	Armour                       *inventory.Armour

	inTransaction, ContainerOpened atomic.Bool
	OpenedWindowID                 atomic.Uint32
	OpenedContainerID              atomic.Uint32
	OpenedWindow                   atomic.Value[*inventory.Inventory]
	handler                        *itemStackRequestHandler
	OpenedPos                      atomic.Value[cube.Pos]
	HeldSlot                       atomic.Uint32
}

func NewManager(client *Client) *ScreenManager {
	m := &ScreenManager{
		c:                 client,
		OffHand:           inventory.New(1, nil),
		Armour:            inventory.NewArmour(nil),
		Inv:               inventory.New(36, nil),
		EnderChest:        inventory.New(27, nil),
		UI:                inventory.New(53, nil),
		inTransaction:     atomic.Bool{},
		ContainerOpened:   atomic.Bool{},
		OpenedWindowID:    atomic.Uint32{},
		OpenedContainerID: atomic.Uint32{},
		HeldSlot:          atomic.Uint32{},
		OpenedWindow:      atomic.Value[*inventory.Inventory]{},
		OpenedPos:         atomic.Value[cube.Pos]{},
		handler:           &itemStackRequestHandler{changes: map[byte]map[byte]changeInfo{}, responseChanges: map[int32]map[*inventory.Inventory]map[byte]responseChange{}},
	}

	AddListener(client, PacketHandler[*packet.ContainerOpen]{
		F: func(client *Client, p *packet.ContainerOpen) error {
			if p.ContainerPosition != (protocol.BlockPos{}) {
				m.OpenedPos.Store(cube.Pos{int((p.ContainerPosition)[0]), int((p.ContainerPosition)[1]), int((p.ContainerPosition)[2])})
			}
			m.ContainerOpened.Store(true)
			m.OpenedWindowID.Store(uint32(p.WindowID))
			m.OpenedContainerID.Store(uint32(p.ContainerType))
			inv, b := m.invByID(int32(p.ContainerType))
			if b {
				m.OpenedWindow.Store(inv)
			}
			return nil
		},
	})
	AddListener(client, PacketHandler[*packet.InventoryContent]{
		F: func(client *Client, p *packet.InventoryContent) error {
			if p.WindowID == protocol.WindowIDInventory {
				for i, instance := range p.Content {
					m.Inv.SetItem(i, StackToItem(instance.Stack))
				}
				return nil
			}
			if p.WindowID == protocol.WindowIDOffHand {
				for i, instance := range p.Content {
					m.OffHand.SetItem(i, StackToItem(instance.Stack))
				}
				return nil
			}
			if p.WindowID == protocol.WindowIDArmour {
				helmet := StackToItem(p.Content[0].Stack)
				chestplate := StackToItem(p.Content[1].Stack)
				leggings := StackToItem(p.Content[2].Stack)
				boots := StackToItem(p.Content[3].Stack)
				m.Armour.Set(helmet, chestplate, leggings, boots)
				return nil
			}
			if p.WindowID == protocol.WindowIDUI {
				for i, instance := range p.Content {
					m.UI.SetItem(i, StackToItem(instance.Stack))
				}
				return nil
			}
			win := m.OpenedWindow.Load()
			for i, instance := range p.Content {
				win.SetItem(i, StackToItem(instance.Stack))
			}

			return nil
		},
	})
	AddListener(client, PacketHandler[*packet.InventorySlot]{
		F: func(client *Client, p *packet.InventorySlot) error {
			if p.WindowID == protocol.WindowIDInventory {
				m.Inv.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
				return nil
			}
			if p.WindowID == protocol.WindowIDOffHand {
				m.OffHand.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
				return nil
			}
			if p.WindowID == protocol.WindowIDArmour {
				switch p.Slot {
				case 0:
					m.Armour.SetHelmet(StackToItem(p.NewItem.Stack))
				case 1:
					m.Armour.SetChestplate(StackToItem(p.NewItem.Stack))
				case 2:
					m.Armour.SetLeggings(StackToItem(p.NewItem.Stack))
				case 3:
					m.Armour.SetBoots(StackToItem(p.NewItem.Stack))
				}
				return nil
			}
			if p.WindowID == protocol.WindowIDUI {
				m.UI.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
				return nil
			}
			win := m.OpenedWindow.Load()
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
		WindowID:   byte(m.OpenedWindowID.Load()),
		ServerSide: false,
	})
	m.OpenedWindowID.Store(0)
}

func (m *ScreenManager) SetCarriedItem(s int) {
	if s > 8 {
		return
	}
	stack, _ := m.Inv.Item(8)

	m.c.Conn.WritePacket(&packet.MobEquipment{
		EntityRuntimeID: m.c.Self.EntityRuntimeID,
		NewItem:         InstanceFromItem(stack),
		InventorySlot:   0,
		HotBarSlot:      byte(s),
		WindowID:        protocol.WindowIDInventory,
	})
	m.HeldSlot.Store(uint32(s))
	//m.HeldItem.Store(stack)
	m.OpenedWindowID.Store(0)
}

// invByID attempts to return an inventory by the ID passed. If found, the inventory is returned and the bool
// returned is true.
func (m *ScreenManager) invByID(id int32) (*inventory.Inventory, bool) {
	switch id {
	case protocol.ContainerCraftingInput, protocol.ContainerCreatedOutput, protocol.ContainerCursor:
		// UI inventory.
		return m.UI, true
	case protocol.ContainerHotBar, protocol.ContainerInventory, protocol.ContainerCombinedHotBarAndInventory:
		// Hotbar 'inventory', rest of inventory, inventory when container is opened.
		return m.Inv, true
	case protocol.ContainerOffhand:
		return m.OffHand, true
	case protocol.ContainerArmor:
		// Armour inventory.
		return m.Armour.Inventory(), true
	case protocol.ContainerLevelEntity:
		if m.ContainerOpened.Load() {
			b := m.c.World().Block(m.OpenedPos.Load())
			if _, chest := b.(block.Chest); chest {
				return m.OpenedWindow.Load(), true
			} else if _, enderChest := b.(block.EnderChest); enderChest {
				return m.OpenedWindow.Load(), true
			}
		}
	case protocol.ContainerBarrel:
		if m.ContainerOpened.Load() {
			if _, barrel := m.c.World().Block(m.OpenedPos.Load()).(block.Barrel); barrel {
				return m.OpenedWindow.Load(), true
			}
		}
	case protocol.ContainerBeaconPayment:
		if m.ContainerOpened.Load() {
			if _, beacon := m.c.World().Block(m.OpenedPos.Load()).(block.Beacon); beacon {
				return m.UI, true
			}
		}
	case protocol.ContainerAnvilInput, protocol.ContainerAnvilMaterial:
		if m.ContainerOpened.Load() {
			if _, anvil := m.c.World().Block(m.OpenedPos.Load()).(block.Anvil); anvil {
				return m.UI, true
			}
		}
	case protocol.ContainerSmithingTableInput, protocol.ContainerSmithingTableMaterial:
		if m.ContainerOpened.Load() {
			if _, smithing := m.c.World().Block(m.OpenedPos.Load()).(block.SmithingTable); smithing {
				return m.UI, true
			}
		}
	case protocol.ContainerLoomInput, protocol.ContainerLoomDye, protocol.ContainerLoomMaterial:
		if m.ContainerOpened.Load() {
			if _, loom := m.c.World().Block(m.OpenedPos.Load()).(block.Loom); loom {
				return m.UI, true
			}
		}
	case protocol.ContainerStonecutterInput:
		if m.ContainerOpened.Load() {
			if _, ok := m.c.World().Block(m.OpenedPos.Load()).(block.Stonecutter); ok {
				return m.UI, true
			}
		}
	case protocol.ContainerGrindstoneInput, protocol.ContainerGrindstoneAdditional:
		if m.ContainerOpened.Load() {
			if _, ok := m.c.World().Block(m.OpenedPos.Load()).(block.Grindstone); ok {
				return m.UI, true
			}
		}
	case protocol.ContainerEnchantingInput, protocol.ContainerEnchantingMaterial:
		if m.ContainerOpened.Load() {
			if _, enchanting := m.c.World().Block(m.OpenedPos.Load()).(block.EnchantingTable); enchanting {
				return m.UI, true
			}
		}
	case protocol.ContainerFurnaceIngredient, protocol.ContainerFurnaceFuel, protocol.ContainerFurnaceResult,
		protocol.ContainerBlastFurnaceIngredient, protocol.ContainerSmokerIngredient:
		if m.ContainerOpened.Load() {
			if _, ok := m.c.World().Block(m.OpenedPos.Load()).(smelter); ok {
				return m.OpenedWindow.Load(), true
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
