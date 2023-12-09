package bot

import (
	_ "embed"
	"git.patyhank.net/falloutBot/bedrocklib/extra"
	"git.patyhank.net/falloutBot/bedrocklib/internal/nbtconv"
	"github.com/df-mc/atomic"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/inventory"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"gopkg.in/square/go-jose.v2/json"
	"slices"
)

const (
	craftingGridSizeSmall   = 4
	craftingGridSizeLarge   = 9
	craftingGridSmallOffset = 28
	craftingGridLargeOffset = 32
	craftingResult          = 50
)

//go:embed data/item_tags.json
var itemTagsFile []byte
var itemTags map[string][]string

func init() {
	json.Unmarshal(itemTagsFile, &itemTags)
}

// TODO: containerID getter
type ScreenManager struct {
	c *Client

	Inv, OffHand, EnderChest, UI *inventory.Inventory
	Armour                       *inventory.Armour

	inTransaction, ContainerOpened atomic.Bool
	OpenedWindowID                 atomic.Uint32
	OpenedContainerID              atomic.Int32
	OpenedWindow                   atomic.Value[*inventory.Inventory]
	handler                        *itemStackRequestHandler
	OpenedPos                      atomic.Value[cube.Pos]
	HeldSlot                       atomic.Uint32
	RequestID                      atomic.Uint32
	HeldItem                       atomic.Value[item.Stack]
	Recipes                        []protocol.Recipe
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
		OpenedContainerID: atomic.Int32{},
		HeldSlot:          atomic.Uint32{},
		RequestID:         atomic.Uint32{},
		OpenedWindow:      atomic.Value[*inventory.Inventory]{},
		OpenedPos:         atomic.Value[cube.Pos]{},
		handler:           &itemStackRequestHandler{changes: map[byte]map[byte]changeInfo{}, responseChanges: map[int32]map[*inventory.Inventory]map[byte]responseChange{}},
	}
	m.OpenedContainerID.Store(-1)

	AddListener(client, PacketHandler[*packet.ContainerOpen]{
		F: func(client *Client, p *packet.ContainerOpen) error {
			if p.ContainerPosition != (protocol.BlockPos{}) {
				m.OpenedPos.Store(cube.Pos{int((p.ContainerPosition)[0]), int((p.ContainerPosition)[1]), int((p.ContainerPosition)[2])})
			}
			m.ContainerOpened.Store(true)
			m.OpenedWindowID.Store(uint32(p.WindowID))
			if p.ContainerPosition != (protocol.BlockPos{}) {
				invBlock, cID := m.openInvBlock(cube.Pos{int((p.ContainerPosition)[0]), int((p.ContainerPosition)[1]), int((p.ContainerPosition)[2])})

				m.OpenedContainerID.Store(int32(cID))
				m.OpenedWindow.Store(invBlock)
			}
			if p.ContainerEntityUniqueID != -1 {
				entity := m.c.Entity.GetEntity(uint64(p.ContainerEntityUniqueID))
				if entity != nil {
					switch m.c.Entity.GetEntity(uint64(p.ContainerEntityUniqueID)).EntityType {
					case "minecraft:villager_v2":
						m.OpenedWindow.Store(inventory.New(3, func(slot int, before, after item.Stack) {}))
					}
				}
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
			if win == nil {
				return nil
			}
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
			if win == nil {
				return nil
			}
			win.SetItem(int(p.Slot), StackToItem(p.NewItem.Stack))
			return nil
		},
	})
	AddListener(client, PacketHandler[*packet.MobEquipment]{
		F: func(client *Client, p *packet.MobEquipment) error {
			if p.EntityRuntimeID == client.Self.EntityRuntimeID {
				m.SetCarriedItem(int(p.HotBarSlot))
			}
			return nil
		},
	})
	AddListener(client, PacketHandler[*packet.CraftingData]{
		F: func(client *Client, p *packet.CraftingData) error {
			if p.ClearRecipes {
				m.Recipes = []protocol.Recipe{}
				m.Recipes = p.Recipes
			}
			return nil
		},
	})

	return m
}

type ActionConfig struct {
	SourceContainerID    int
	RemoteContainerID    int
	GuessRemoteItemStack item.Stack
}

func (m *ScreenManager) SwapItemAction(origin, destination int, config ...ActionConfig) protocol.StackRequestAction {
	if len(config) == 0 && (m.OpenedContainerID.Load() == -1) {
		return nil
	}
	p := &protocol.SwapStackRequestAction{}
	if origin >= m.Inv.Size() {
		p.Source = protocol.StackRequestSlotInfo{
			ContainerID:    byte(m.OpenedContainerID.Load()),
			Slot:           byte(origin),
			StackNetworkID: -1,
		}
	} else {
		p.Source = protocol.StackRequestSlotInfo{
			ContainerID:    byte(protocol.ContainerCombinedHotBarAndInventory),
			Slot:           byte(origin),
			StackNetworkID: -1,
		}
	}
	if destination < m.Inv.Size() {
		p.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    byte(m.OpenedContainerID.Load()),
			Slot:           byte(destination),
			StackNetworkID: -1,
		}
	} else {
		p.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    byte(protocol.ContainerCombinedHotBarAndInventory),
			Slot:           byte(destination),
			StackNetworkID: -1,
		}
	}
	return p
}
func (m *ScreenManager) AutoCraftAction(recipeID uint32) protocol.StackRequestAction {
	p := &protocol.AutoCraftRecipeStackRequestAction{
		RecipeNetworkID: recipeID,
	}
	return p
}
func (m *ScreenManager) SearchSlotInInv(stack world.Item, totalCount int, perSlot int) map[int]int {
	y := map[int]int{}
	for i, slot := range m.Inv.Slots() {
		//if slices.Contains(excludeSlots, i) {
		//	continue
		//}
		if slot.Empty() {
			continue
		}
		if slot.Count() < perSlot {
			continue
		}
		if stack == slot.Item() {
			y[i] = min(slot.Count(), totalCount)
			totalCount -= min(slot.Count(), totalCount)
		}

		if totalCount <= 0 {
			break
		}
	}
	if totalCount > 0 {
		return nil
	}

	return y
}
func (m *ScreenManager) SearchSlotInInvTag(stackTag string, totalCount int, perSlot int) map[int]int {
	tagItems, ok := itemTags[stackTag]
	if !ok {
		return nil
	}
	y := map[int]int{}
	for i, slot := range m.Inv.Slots() {
		if slot.Empty() {
			continue
		}

		name, _ := slot.Item().EncodeItem()
		if slot.Count() < perSlot {
			continue
		}
		if slices.Contains(tagItems, name) {
			y[i] = min(slot.Count(), totalCount)
			totalCount -= min(slot.Count(), totalCount)
		}
		if totalCount <= 0 {
			break
		}
	}
	if totalCount > 0 {
		return nil
	}

	return y
}

func (m *ScreenManager) GetAutomaticCraftID(itt world.Item, craftCount int) uint32 {
	name, meta := itt.EncodeItem()
A:
	for _, recipe := range m.Recipes {
		switch r := recipe.(type) {
		case *protocol.ShapelessRecipe:
			it := StackToItem(r.Output[0])
			oname, ometa := it.Item().EncodeItem()
			if oname != name || meta != ometa {
				continue
			}
			mnTag := map[string]int{}
			mnTagSlot := map[string][]int{}
			mnItem := map[world.Item]int{}
			mnItemSlot := map[world.Item][]int{}

			for i, input := range r.Input {
				switch d := input.Descriptor.(type) {
				case *protocol.ItemTagItemDescriptor:
					mnTag[d.Tag] += (int(input.Count) * craftCount)
					mnTagSlot[d.Tag] = append(mnTagSlot[d.Tag], i)
				case *protocol.DefaultItemDescriptor:
					it, _ := world.ItemByRuntimeID(int32(d.NetworkID), d.MetadataValue)
					mnItem[it] += (int(input.Count) * craftCount)
					mnItemSlot[it] = append(mnItemSlot[it], i)
				}
			}

			for tag, count := range mnTag {
				slots := mnTagSlot[tag]
				perSlot := count / len(slots)
				inv := m.SearchSlotInInvTag(tag, count, perSlot)
				if inv == nil {
					continue A
				}
			}

			for it, count := range mnItem {
				slots := mnItemSlot[it]
				perSlot := count / len(slots)
				inv := m.SearchSlotInInv(it, count, perSlot)
				if inv == nil {
					continue A
				}
			}
			return r.RecipeNetworkID
		case *protocol.ShapedRecipe:
			it := StackToItem(r.Output[0])
			oname, ometa := it.Item().EncodeItem()
			if oname != name || meta != ometa {
				continue
			}
			mnTag := map[string]int{}
			mnTagSlot := map[string][]int{}
			mnItem := map[world.Item]int{}
			mnItemSlot := map[world.Item][]int{}

			for i, input := range r.Input {
				switch d := input.Descriptor.(type) {
				case *protocol.ItemTagItemDescriptor:
					mnTag[d.Tag] += (int(input.Count) * craftCount)
					mnTagSlot[d.Tag] = append(mnTagSlot[d.Tag], i)
				case *protocol.DefaultItemDescriptor:
					it, _ := world.ItemByRuntimeID(int32(d.NetworkID), d.MetadataValue)
					mnItem[it] += (int(input.Count) * craftCount)
					mnItemSlot[it] = append(mnItemSlot[it], i)
				}
			}

			for tag, count := range mnTag {
				slots := mnTagSlot[tag]
				perSlot := count / len(slots)
				inv := m.SearchSlotInInvTag(tag, count, perSlot)
				if inv == nil {
					continue A
				}
			}

			for it, count := range mnItem {
				slots := mnItemSlot[it]
				perSlot := count / len(slots)
				inv := m.SearchSlotInInv(it, count, perSlot)
				if inv == nil {
					continue A
				}
			}
			return r.RecipeNetworkID
		}
	}

	return 0
}
func (m *ScreenManager) AutomaticCraftingActions(recipeID uint32, craftCount int) []protocol.StackRequestAction {
	var as []protocol.StackRequestAction
	var outputItem item.Stack
	mnTag := map[string]int{}
	mnTagSlot := map[string][]int{}
	mnItem := map[world.Item]int{}
	mnItemSlot := map[world.Item][]int{}

R:
	for _, recipe := range m.Recipes {
		switch r := recipe.(type) {
		case *protocol.ShapelessRecipe:
			if r.RecipeNetworkID == recipeID {
				{
					p := &protocol.AutoCraftRecipeStackRequestAction{
						RecipeNetworkID: recipeID,
						TimesCrafted:    byte(craftCount),
						Ingredients:     r.Input,
					}
					as = append(as, p)
				}
				{
					p := &protocol.CraftResultsDeprecatedStackRequestAction{
						TimesCrafted: byte(craftCount),
						ResultItems:  r.Output,
					}
					as = append(as, p)
				}
				output := StackToItem(r.Output[0])
				output = output.Grow(output.Count() * (craftCount - 1))
				outputItem = output

				for i, input := range r.Input {
					switch d := input.Descriptor.(type) {
					case *protocol.ItemTagItemDescriptor:
						mnTag[d.Tag] += (int(input.Count) * craftCount)
						mnTagSlot[d.Tag] = append(mnTagSlot[d.Tag], i)
					case *protocol.DefaultItemDescriptor:
						it, _ := world.ItemByRuntimeID(int32(d.NetworkID), d.MetadataValue)
						mnItem[it] += (int(input.Count) * craftCount)
						mnItemSlot[it] = append(mnItemSlot[it], i)
					}
				}
				for tag, count := range mnTag {
					slots := mnTagSlot[tag]
					perSlot := count / len(slots)
					inv := m.SearchSlotInInvTag(tag, count, perSlot)
					if inv != nil {
						for s := 0; s < len(slots); s++ {
							for slot, count := range inv {
								if count < perSlot {
									continue
								}
								as = append(as, m.ConsumeAction(slot, perSlot, ActionConfig{SourceContainerID: protocol.ContainerCombinedHotBarAndInventory}))
								inv[slot] -= perSlot
							}
						}
					}
				}
				for it, count := range mnItem {
					slots := mnItemSlot[it]
					perSlot := count / len(slots)
					inv := m.SearchSlotInInv(it, count, perSlot)
					if inv != nil {
						for s := 0; s < len(slots); s++ {
							for slot, count := range inv {
								if count < perSlot {
									continue
								}
								as = append(as, m.ConsumeAction(slot, perSlot, ActionConfig{SourceContainerID: protocol.ContainerCombinedHotBarAndInventory}))
								inv[slot] -= perSlot
							}
						}
					}
				}
				break R
			}
		case *protocol.ShapedRecipe:
			if r.RecipeNetworkID == recipeID {
				{
					p := &protocol.AutoCraftRecipeStackRequestAction{
						RecipeNetworkID: recipeID,
						TimesCrafted:    byte(craftCount),
						Ingredients:     r.Input,
					}
					as = append(as, p)
				}
				{
					p := &protocol.CraftResultsDeprecatedStackRequestAction{
						TimesCrafted: byte(craftCount),
						ResultItems:  r.Output,
					}
					as = append(as, p)
				}
				output := StackToItem(r.Output[0])
				output = output.Grow(output.Count() * (craftCount - 1))
				outputItem = output

				for i, input := range r.Input {
					switch d := input.Descriptor.(type) {
					case *protocol.ItemTagItemDescriptor:
						mnTag[d.Tag] += (int(input.Count) * craftCount)
						mnTagSlot[d.Tag] = append(mnTagSlot[d.Tag], i)
					case *protocol.DefaultItemDescriptor:
						it, _ := world.ItemByRuntimeID(int32(d.NetworkID), d.MetadataValue)
						mnItem[it] += (int(input.Count) * craftCount)
						mnItemSlot[it] = append(mnItemSlot[it], i)
					}
				}
				for tag, count := range mnTag {
					slots := mnTagSlot[tag]
					perSlot := count / len(slots)
					inv := m.SearchSlotInInvTag(tag, count, perSlot)
					if inv != nil {
						for s := 0; s < len(slots); s++ {
							for slot, count := range inv {
								if count < perSlot {
									continue
								}
								as = append(as, m.ConsumeAction(slot, perSlot, ActionConfig{SourceContainerID: protocol.ContainerCombinedHotBarAndInventory}))
								inv[slot] -= perSlot
							}
						}
					}
				}
				for it, count := range mnItem {
					slots := mnItemSlot[it]
					perSlot := count / len(slots)
					inv := m.SearchSlotInInv(it, count, perSlot)
					if inv != nil {
						for s := 0; s < len(slots); s++ {
							for slot, count := range inv {
								if count < perSlot {
									continue
								}
								as = append(as, m.ConsumeAction(slot, perSlot, ActionConfig{SourceContainerID: protocol.ContainerCombinedHotBarAndInventory}))
								inv[slot] -= perSlot
							}
						}
					}
				}
				break R
			}
		}
	}
	count := outputItem.Count()
	maxCount := outputItem.MaxCount()

	var markedSlots []int
	for count > 0 {
		var first int = -1
		for slot, st := range m.Inv.Slots() {
			if slices.Contains(markedSlots, slot) {
				continue
			}
			if st.Empty() || outputItem.Empty() {
				continue
			}
			name, meta := st.Item().EncodeItem()
			oname, ometa := outputItem.Item().EncodeItem()
			if (oname == name && meta == ometa) && st.Count() != st.MaxCount() {
				first = slot
				break
			}
		}
		if first == -1 {
			for slot, st := range m.Inv.Slots() {
				if slices.Contains(markedSlots, slot) {
					continue
				}
				if st.Empty() {
					first = slot
					break
				}
			}
		}
		if first == -1 {
			break
		}
		markedSlots = append(markedSlots, first)
		i, _ := m.Inv.Item(first)
		storeCount := min(maxCount-i.Count(), count)
		p := &protocol.PlaceStackRequestAction{}
		p.Count = byte(storeCount)
		p.Source = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerCreatedOutput,
			Slot:           craftingResult,
			StackNetworkID: -1,
		}
		p.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    protocol.ContainerCombinedHotBarAndInventory,
			Slot:           byte(first),
			StackNetworkID: -1,
		}
		as = append(as, p)
		count -= storeCount
	}

	return as
}
func (m *ScreenManager) ConsumeAction(slot int, count int, config ...ActionConfig) protocol.StackRequestAction {
	cID := m.OpenedContainerID.Load()
	if len(config) > 0 {
		cID = int32(config[0].SourceContainerID)
	}

	p := &protocol.ConsumeStackRequestAction{}
	p.DestroyStackRequestAction = protocol.DestroyStackRequestAction{
		Count: byte(count),
		Source: protocol.StackRequestSlotInfo{
			ContainerID:    byte(cID),
			Slot:           byte(slot),
			StackNetworkID: -1,
		},
	}
	return p
}
func (m *ScreenManager) StoreItemAction(origin int, up bool, config ...ActionConfig) []protocol.StackRequestAction {
	if len(config) == 0 && (m.OpenedContainerID.Load() == -1) {
		return nil
	}
	var aconfig ActionConfig
	if len(config) > 0 {
		aconfig = config[0]
	}

	var actions []protocol.StackRequestAction
	if up {
		stack, _ := m.Inv.Item(origin)
		count := stack.Count()
		maxCount := stack.MaxCount()
		w := m.OpenedWindow.Load()
		var markedSlots []int
		for count > 0 {
			var first int = -1
			for slot, st := range w.Slots() {
				if slices.Contains(markedSlots, slot) {
					continue
				}
				if st.Empty() || stack.Empty() {
					continue
				}
				name, meta := st.Item().EncodeItem()
				oname, ometa := st.Item().EncodeItem()
				if (oname == name && meta == ometa) && st.Count() != st.MaxCount() {
					first = slot
					break
				}
			}
			if first == -1 {
				for slot, st := range w.Slots() {
					if slices.Contains(markedSlots, slot) {
						continue
					}
					if st.Empty() {
						first = slot
						break
					}
				}
			}
			if first == -1 {
				break
			}
			markedSlots = append(markedSlots, first)
			i, _ := w.Item(first)
			storeCount := min(maxCount-i.Count(), count)
			actions = append(actions, m.PlaceItemAction(origin, m.Inv.Size()+first, byte(storeCount), aconfig))
			count -= storeCount
		}
	} else {
		var stack item.Stack
		if aconfig.GuessRemoteItemStack.Empty() {
			w := m.OpenedWindow.Load()
			stack, _ = w.Item(origin)
		} else {
			stack = aconfig.GuessRemoteItemStack
		}
		count := stack.Count()
		maxCount := stack.MaxCount()

		var markedSlots []int

		for count > 0 {
			var first int = -1
			for slot, st := range m.Inv.Slots() {
				if slices.Contains(markedSlots, slot) {
					continue
				}
				if st.Empty() || stack.Empty() {
					continue
				}
				name, meta := stack.Item().EncodeItem()
				oname, ometa := st.Item().EncodeItem()
				if (oname == name && meta == ometa) && st.Count() != st.MaxCount() {
					first = slot
					break
				}
			}
			if first == -1 {
				for slot, st := range m.Inv.Slots() {
					if slices.Contains(markedSlots, slot) {
						continue
					}
					if st.Empty() {
						first = slot
						break
					}
				}
			}
			if first == -1 {
				break
			}
			markedSlots = append(markedSlots, first)
			i, _ := m.Inv.Item(first)
			storeCount := min(maxCount-i.Count(), count)
			actions = append(actions, m.PlaceItemAction(origin+m.Inv.Size(), first, byte(storeCount), aconfig))
			count -= storeCount
		}
	}
	return actions
}

//// TakeItemAction Taking Item (slot -1 to cursor)
//func (m *ScreenManager) TakeItemAction(origin, destination int, count byte, config ...ActionConfig) protocol.StackRequestAction {
//	if len(config) == 0 && (m.OpenedContainerID.Load() == -1) {
//		return nil
//	}
//	p := &protocol.TakeStackRequestAction{}
//
//	p.Count = count
//
//	if origin >= m.Inv.Size() {
//		p.Source = protocol.StackRequestSlotInfo{
//			ContainerID:    byte(m.OpenedContainerID.Load()),
//			Slot:           byte(origin - m.Inv.Size()),
//			StackNetworkID: -1,
//		}
//	} else {
//		switch origin {
//		case -1:
//			p.Source = protocol.StackRequestSlotInfo{
//				ContainerID:    byte(protocol.ContainerCursor),
//				Slot:           byte(0),
//				StackNetworkID: -1,
//			}
//		default:
//			p.Source = protocol.StackRequestSlotInfo{
//				ContainerID:    byte(protocol.ContainerCombinedHotBarAndInventory),
//				Slot:           byte(origin),
//				StackNetworkID: -1,
//			}
//		}
//	}
//	if destination >= m.Inv.Size() {
//		p.Destination = protocol.StackRequestSlotInfo{
//			ContainerID:    byte(m.OpenedContainerID.Load()),
//			Slot:           byte(destination - m.Inv.Size()),
//			StackNetworkID: -1,
//		}
//	} else {
//		switch origin {
//		case -1:
//			p.Destination = protocol.StackRequestSlotInfo{
//				ContainerID:    byte(protocol.ContainerCursor),
//				Slot:           byte(0),
//				StackNetworkID: -1,
//			}
//		default:
//			p.Destination = protocol.StackRequestSlotInfo{
//				ContainerID:    byte(protocol.ContainerCombinedHotBarAndInventory),
//				Slot:           byte(destination),
//				StackNetworkID: -1,
//			}
//		}
//	}
//	return p
//}

func (m *ScreenManager) PlaceItemAction(origin, destination int, count byte, config ...ActionConfig) protocol.StackRequestAction {
	if len(config) == 0 && (m.OpenedContainerID.Load() == -1) {
		return nil
	}
	remoteContainerID := m.OpenedContainerID.Load()
	if len(config) > 0 {
		remoteContainerID = int32(config[0].RemoteContainerID)
	}
	p := &protocol.PlaceStackRequestAction{}

	p.Count = count
	if origin >= m.Inv.Size() {
		p.Source = protocol.StackRequestSlotInfo{
			ContainerID:    byte(remoteContainerID),
			Slot:           byte(origin - m.Inv.Size()),
			StackNetworkID: -1,
		}
	} else {
		switch origin {
		case -1:
			p.Source = protocol.StackRequestSlotInfo{
				ContainerID:    byte(protocol.ContainerCursor),
				Slot:           byte(0),
				StackNetworkID: -1,
			}
		default:
			p.Source = protocol.StackRequestSlotInfo{
				ContainerID:    byte(protocol.ContainerCombinedHotBarAndInventory),
				Slot:           byte(origin),
				StackNetworkID: -1,
			}
		}
	}
	if destination >= m.Inv.Size() {
		p.Destination = protocol.StackRequestSlotInfo{
			ContainerID:    byte(remoteContainerID),
			Slot:           byte(destination - m.Inv.Size()),
			StackNetworkID: -1,
		}
	} else {
		switch origin {
		case -1:
			p.Destination = protocol.StackRequestSlotInfo{
				ContainerID:    byte(protocol.ContainerCursor),
				Slot:           byte(0),
				StackNetworkID: -1,
			}
		default:
			p.Destination = protocol.StackRequestSlotInfo{
				ContainerID:    byte(protocol.ContainerCombinedHotBarAndInventory),
				Slot:           byte(destination),
				StackNetworkID: -1,
			}
		}
	}
	return p
}

func (m *ScreenManager) PackingRequestAction(req ...protocol.StackRequestAction) protocol.ItemStackRequest {
	var r protocol.ItemStackRequest
	r.RequestID = int32(m.RequestID.Inc())
	for _, action := range req {
		r.Actions = append(r.Actions, action)
	}
	return r
}

func (m *ScreenManager) PackingRequestPacket(req ...protocol.ItemStackRequest) *packet.ItemStackRequest {
	r := &packet.ItemStackRequest{
		Requests: append([]protocol.ItemStackRequest{}, req...),
	}
	return r
}

// SendContainerClick 驗證並傳送視窗點擊封包
func (m *ScreenManager) SendContainerClick(request *packet.ItemStackRequest) error {
	err := m.handler.Handle(request, m)
	if err != nil {
		return err
	}
	return m.c.Conn.WritePacket(request)
}

// CloseCurrentWindow 關閉目前視窗
func (m *ScreenManager) CloseCurrentWindow() {
	m.c.Conn.WritePacket(&packet.ContainerClose{
		WindowID:   byte(m.OpenedWindowID.Load()),
		ServerSide: false,
	})
	m.OpenedWindowID.Store(0)
}

// SetCarriedItem 設定手持物品(hotbar)格數
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
	m.HeldItem.Store(stack)
	m.OpenedWindowID.Store(0)
}

// openInv attempts to return an inventory by the ID passed. If found, the inventory is returned and the bool
// returned is true.
func (m *ScreenManager) openInvBlock(pos cube.Pos) (*inventory.Inventory, int) {
	b := m.c.World().Block(pos)
	be := m.c.World().BlockEntity(pos)

	if _, chest := b.(block.Chest); chest {
		if _, pairing := be["pairx"]; pairing {
			return inventory.New(54, func(slot int, before, after item.Stack) {}), protocol.ContainerLevelEntity
		} else {
			return inventory.New(27, func(slot int, before, after item.Stack) {}), protocol.ContainerLevelEntity
		}
	}
	if _, barrel := b.(block.Barrel); barrel {
		return inventory.New(27, func(slot int, before, after item.Stack) {}), protocol.ContainerBarrel
	}
	if _, shulker := b.(extra.ShulkerBox); shulker {
		return inventory.New(27, func(slot int, before, after item.Stack) {}), protocol.ContainerShulkerBox
	}
	if _, anvil := b.(block.Anvil); anvil {
		return inventory.New(27, func(slot int, before, after item.Stack) {}), -1
	}
	if _, furnace := b.(block.Furnace); furnace {
		return inventory.New(3, func(slot int, before, after item.Stack) {}), -1
	}
	if _, furnace := b.(block.Smoker); furnace {
		return inventory.New(3, func(slot int, before, after item.Stack) {}), -1
	}
	if _, furnace := b.(block.BlastFurnace); furnace {
		return inventory.New(3, func(slot int, before, after item.Stack) {}), -1
	}
	if _, smith := b.(block.SmithingTable); smith {
		return inventory.New(4, func(slot int, before, after item.Stack) {}), -1
	}
	if _, smith := b.(block.CraftingTable); smith {
		return inventory.New(10, func(slot int, before, after item.Stack) {}), -1
	}
	if _, smith := b.(block.Grindstone); smith {
		return inventory.New(3, func(slot int, before, after item.Stack) {}), -1
	}

	return nil, -1
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
			return m.OpenedWindow.Load(), true
		}
	case protocol.ContainerBarrel:
		if m.ContainerOpened.Load() {
			return m.OpenedWindow.Load(), true
		}
	case protocol.ContainerBeaconPayment:
		if m.ContainerOpened.Load() {
			return m.UI, true
		}
	case protocol.ContainerAnvilInput, protocol.ContainerAnvilMaterial:
		if m.ContainerOpened.Load() {
			return m.UI, true
			if _, anvil := m.c.World().Block(m.OpenedPos.Load()).(block.Anvil); anvil {
			}
		}
	case protocol.ContainerSmithingTableInput, protocol.ContainerSmithingTableMaterial:
		if m.ContainerOpened.Load() {
			return m.UI, true
			if _, smithing := m.c.World().Block(m.OpenedPos.Load()).(block.SmithingTable); smithing {
			}
		}
	case protocol.ContainerLoomInput, protocol.ContainerLoomDye, protocol.ContainerLoomMaterial:
		if m.ContainerOpened.Load() {
			return m.UI, true
			if _, loom := m.c.World().Block(m.OpenedPos.Load()).(block.Loom); loom {
			}
		}
	case protocol.ContainerStonecutterInput:
		if m.ContainerOpened.Load() {
			return m.UI, true
			if _, ok := m.c.World().Block(m.OpenedPos.Load()).(block.Stonecutter); ok {
			}
		}
	case protocol.ContainerGrindstoneInput, protocol.ContainerGrindstoneAdditional:
		if m.ContainerOpened.Load() {
			return m.UI, true
			if _, ok := m.c.World().Block(m.OpenedPos.Load()).(block.Grindstone); ok {
			}
		}
	case protocol.ContainerEnchantingInput, protocol.ContainerEnchantingMaterial:
		if m.ContainerOpened.Load() {
			return m.UI, true
			if _, enchanting := m.c.World().Block(m.OpenedPos.Load()).(block.EnchantingTable); enchanting {
			}
		}
	case protocol.ContainerFurnaceIngredient, protocol.ContainerFurnaceFuel, protocol.ContainerFurnaceResult,
		protocol.ContainerBlastFurnaceIngredient, protocol.ContainerSmokerIngredient:
		if m.ContainerOpened.Load() {
			return m.OpenedWindow.Load(), true
			if _, ok := m.c.World().Block(m.OpenedPos.Load()).(smelter); ok {
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
	if damage, ok := it.NBTData["Damage"].(int32); ok {
		it.NBTData["Damage"] = damage - 1
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
