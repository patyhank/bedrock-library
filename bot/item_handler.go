package bot

import (
	"fmt"
	"github.com/df-mc/dragonfly/server/event"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/inventory"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"math"
	"time"
	_ "unsafe"
)

// itemStackRequestHandler handles the ItemStackRequest packet. It handles the actions done within the
// inventory.
type itemStackRequestHandler struct {
	currentRequest int32

	changes         map[byte]map[byte]changeInfo
	responseChanges map[int32]map[*inventory.Inventory]map[byte]responseChange

	pendingResults []item.Stack

	current       time.Time
	ignoreDestroy bool
}

// responseChange represents a change in a specific item stack response. It holds the timestamp of the
// response which is used to get rid of changes that the client will have received.
type responseChange struct {
	id        int32
	timestamp time.Time
}

// changeInfo holds information on a slot change initiated by an item stack request. It holds both the new and the old
// item information and is used for reverting and verifying.
type changeInfo struct {
	after  protocol.StackResponseSlotInfo
	before item.Stack
}

// Handle ...
func (h *itemStackRequestHandler) Handle(p packet.Packet, s *ScreenManager) error {
	pk := p.(*packet.ItemStackRequest)
	h.current = time.Now()

	s.inTransaction.Store(true)
	defer s.inTransaction.Store(false)

	for _, req := range pk.Requests {
		if err := h.handleRequest(req, s); err != nil {
			return err
		}
	}
	return nil
}

// handleRequest resolves a single item stack request from the client.
func (h *itemStackRequestHandler) handleRequest(req protocol.ItemStackRequest, s *ScreenManager) (err error) {
	h.currentRequest = req.RequestID
	defer func() {
		if err != nil {
			h.reject(req.RequestID, s)
			return
		}
		//h.resolve(req.RequestID, s)
		h.ignoreDestroy = false
	}()

	for _, action := range req.Actions {
		switch a := action.(type) {
		case *protocol.TakeStackRequestAction:
			err = h.handleTake(a, s)
		case *protocol.PlaceStackRequestAction:
			err = h.handlePlace(a, s)
		case *protocol.SwapStackRequestAction:
			err = h.handleSwap(a, s)
		case *protocol.DestroyStackRequestAction:
			err = h.handleDestroy(a, s)
		case *protocol.DropStackRequestAction:
			err = h.handleDrop(a, s)
		//case *protocol.BeaconPaymentStackRequestAction:
		//	err = h.handleBeaconPayment(a, s)
		//case *protocol.CraftRecipeStackRequestAction:
		//	if s.containerOpened.Load() {
		//		var special bool
		//		switch s.c.World().Block(s.openedPos.Load()).(type) {
		//		case block.SmithingTable:
		//			err, special = h.handleSmithing(a, s), true
		//		case block.Stonecutter:
		//			err, special = h.handleStonecutting(a, s), true
		//		case block.EnchantingTable:
		//			err, special = h.handleEnchant(a, s), true
		//		}
		//		if special {
		//			// This was a "special action" and was handled, so we can move onto the next action.
		//			break
		//		}
		//	}
		//	err = h.handleCraft(a, s)
		//case *protocol.AutoCraftRecipeStackRequestAction:
		//	err = h.handleAutoCraft(a, s)
		//case *protocol.CraftRecipeOptionalStackRequestAction:
		//	err = h.handleCraftRecipeOptional(a, s, req.FilterStrings)
		//case *protocol.CraftLoomRecipeStackRequestAction:
		//	err = h.handleLoomCraft(a, s)
		//case *protocol.CraftGrindstoneRecipeStackRequestAction:
		//	err = h.handleGrindstoneCraft(s)
		//case *protocol.CraftCreativeStackRequestAction:
		//	err = h.handleCreativeCraft(a, s)
		case *protocol.MineBlockStackRequestAction:
			err = h.handleMineBlock(a, s)
		case *protocol.CreateStackRequestAction:
			err = h.handleCreate(a, s)
		case *protocol.ConsumeStackRequestAction, *protocol.CraftResultsDeprecatedStackRequestAction:
			// Don't do anything with this.
		default:
			return nil
		}
		if err != nil {
			err = fmt.Errorf("%T: %w", action, err)
			return
		}
	}
	return
}

// handleTake handles a Take stack request action.
func (h *itemStackRequestHandler) handleTake(a *protocol.TakeStackRequestAction, s *ScreenManager) error {
	return h.handleTransfer(a.Source, a.Destination, a.Count, s)
}

// handlePlace handles a Place stack request action.
func (h *itemStackRequestHandler) handlePlace(a *protocol.PlaceStackRequestAction, s *ScreenManager) error {
	return h.handleTransfer(a.Source, a.Destination, a.Count, s)
}

// handleTransfer handles the transferring of x count from a source slot to a destination slot.
func (h *itemStackRequestHandler) handleTransfer(from, to protocol.StackRequestSlotInfo, count byte, s *ScreenManager) error {
	if err := h.verifySlots(s, from, to); err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	i, _ := h.itemInSlot(from, s)
	dest, _ := h.itemInSlot(to, s)
	if !i.Comparable(dest) {
		return fmt.Errorf("client tried transferring %v to %v, but the stacks are incomparable", i, dest)
	}
	if i.Count() < int(count) {
		return fmt.Errorf("client tried subtracting %v from item count, but there are only %v", count, i.Count())
	}
	if (dest.Count()+int(count) > dest.MaxCount()) && !dest.Empty() {
		return fmt.Errorf("client tried adding %v to item count %v, but max is %v", count, dest.Count(), dest.MaxCount())
	}
	if dest.Empty() {
		dest = i.Grow(-math.MaxInt32)
	}

	//invA, _ := s.invByID(int32(from.ContainerID))
	//invB, _ := s.invByID(int32(to.ContainerID))
	//
	//ctx := event.C()
	//_ = call(ctx, int(from.Slot), i.Grow(int(count)-i.Count()), invA.Handler().HandleTake)
	//err := call(ctx, int(to.Slot), i.Grow(int(count)-i.Count()), invB.Handler().HandlePlace)
	//if err != nil {
	//	return err
	//}

	h.setItemInSlot(from, i.Grow(-int(count)), s)
	h.setItemInSlot(to, dest.Grow(int(count)), s)
	return nil
}

// handleSwap handles a Swap stack request action.
func (h *itemStackRequestHandler) handleSwap(a *protocol.SwapStackRequestAction, s *ScreenManager) error {
	if err := h.verifySlots(s, a.Source, a.Destination); err != nil {
		return fmt.Errorf("slot out of sync: %w", err)
	}
	i, _ := h.itemInSlot(a.Source, s)
	dest, _ := h.itemInSlot(a.Destination, s)

	//invA, _ := s.invByID(int32(a.Source.Container.ContainerID))
	//invB, _ := s.invByID(int32(a.Destination.Container.ContainerID))

	//ctx := event.C(inventory.Holder(s))
	//_ = call(ctx, int(a.Source.Slot), i, invA.Handler().HandleTake)
	//_ = call(ctx, int(a.Source.Slot), dest, invA.Handler().HandlePlace)
	//_ = call(ctx, int(a.Destination.Slot), dest, invB.Handler().HandleTake)
	//err := call(ctx, int(a.Destination.Slot), i, invB.Handler().HandlePlace)
	//if err != nil {
	//	return err
	//}

	h.setItemInSlot(a.Source, dest, s)
	h.setItemInSlot(a.Destination, i, s)
	return nil
}

// handleDestroy handles the destroying of an item by moving it into the creative inventory.
func (h *itemStackRequestHandler) handleDestroy(a *protocol.DestroyStackRequestAction, s *ScreenManager) error {
	if h.ignoreDestroy {
		return nil
	}
	if err := h.verifySlot(a.Source, s); err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	i, _ := h.itemInSlot(a.Source, s)
	if i.Count() < int(a.Count) {
		return fmt.Errorf("client attempted to destroy %v items, but only %v present", a.Count, i.Count())
	}

	h.setItemInSlot(a.Source, i.Grow(-int(a.Count)), s)
	return nil
}

// handleDrop handles the dropping of an item by moving it outside the inventory while having the
// inventory opened.
func (h *itemStackRequestHandler) handleDrop(a *protocol.DropStackRequestAction, s *ScreenManager) error {
	if err := h.verifySlot(a.Source, s); err != nil {
		return fmt.Errorf("source slot out of sync: %w", err)
	}
	i, _ := h.itemInSlot(a.Source, s)
	if i.Count() < int(a.Count) {
		return fmt.Errorf("client attempted to drop %v items, but only %v present", a.Count, i.Count())
	}

	inv, _ := s.invByID(int32(a.Source.Container.ContainerID))
	if err := call(event.C(inventory.Holder(s)), int(a.Source.Slot), i.Grow(int(a.Count)-i.Count()), inv.Handler().HandleDrop); err != nil {
		return err
	}

	h.setItemInSlot(a.Source, i.Grow(-i.Grow(int(a.Count)-i.Count()).Count()), s)
	return nil
}

// handleMineBlock handles the action associated with a block being mined by the player. This seems to be a workaround
// by Mojang to deal with the durability changes client-side.
func (h *itemStackRequestHandler) handleMineBlock(a *protocol.MineBlockStackRequestAction, s *ScreenManager) error {
	slot := protocol.StackRequestSlotInfo{
		Container:      protocol.FullContainerName{ContainerID: protocol.ContainerInventory},
		Slot:           byte(a.HotbarSlot),
		StackNetworkID: a.StackNetworkID,
	}
	if err := h.verifySlot(slot, s); err != nil {
		return err
	}

	// Update the slots through ItemStackResponses, don't actually do anything special with this action.
	i, _ := h.itemInSlot(slot, s)
	h.setItemInSlot(slot, i, s)
	return nil
}

// handleCreate handles the CreateStackRequestAction sent by the client when a recipe outputs more than one item. It
// contains a result slot, which should map to one of the output items. From there, the server should create the relevant
// output as usual.
func (h *itemStackRequestHandler) handleCreate(a *protocol.CreateStackRequestAction, s *ScreenManager) error {
	slot := int(a.ResultsSlot)
	if len(h.pendingResults) < slot {
		return fmt.Errorf("invalid pending result slot: %v", a.ResultsSlot)
	}

	res := h.pendingResults[slot]
	if res.Empty() {
		return fmt.Errorf("tried duplicating created result: %v", slot)
	}
	h.pendingResults[slot] = item.Stack{}

	return nil
}

// defaultCreation represents the CreateStackRequestAction used for single-result crafts.
var defaultCreation = &protocol.CreateStackRequestAction{}

// createResults creates a new craft result and adds it to the list of pending craft results.
func (h *itemStackRequestHandler) createResults(s *ScreenManager, result ...item.Stack) error {
	h.pendingResults = append(h.pendingResults, result...)
	if len(result) > 1 {
		// With multiple results, the client notifies the server on when to create the results.
		return nil
	}
	return h.handleCreate(defaultCreation, s)
}

// verifySlots verifies a list of slots passed.
func (h *itemStackRequestHandler) verifySlots(s *ScreenManager, slots ...protocol.StackRequestSlotInfo) error {
	for _, slot := range slots {
		if err := h.verifySlot(slot, s); err != nil {
			return err
		}
	}
	return nil
}

// verifySlot checks if the slot passed by the client is the same as that expected by the server.
func (h *itemStackRequestHandler) verifySlot(slot protocol.StackRequestSlotInfo, s *ScreenManager) error {
	return nil
	if err := h.tryAcknowledgeChanges(s, slot); err != nil {
		return err
	}
	if len(h.responseChanges) > 256 {
		return fmt.Errorf("too many unacknowledged request slot changes")
	}
	inv, _ := s.invByID(int32(slot.Container.ContainerID))

	i, err := h.itemInSlot(slot, s)
	if err != nil {
		return err
	}
	clientID, err := h.resolveID(inv, slot)
	if err != nil {
		return err
	}
	// The client seems to send negative stack network IDs for predictions, which we can ignore. We'll simply
	// override this network ID later.
	if id := item_id(i); id != clientID {
		return fmt.Errorf("stack ID mismatch: client expected %v, but server had %v", clientID, id)
	}
	return nil
}

// resolveID resolves the stack network ID in the slot passed. If it is negative, it points to an earlier
// request, in which case it will look it up in the changes of an earlier response to a request to find the
// actual stack network ID in the slot. If it is positive, the ID will be returned again.
func (h *itemStackRequestHandler) resolveID(inv *inventory.Inventory, slot protocol.StackRequestSlotInfo) (int32, error) {
	if slot.StackNetworkID >= 0 {
		return slot.StackNetworkID, nil
	}
	containerChanges, ok := h.responseChanges[slot.StackNetworkID]
	if !ok {
		return 0, fmt.Errorf("slot pointed to stack request %v, but request could not be found", slot.StackNetworkID)
	}
	changes, ok := containerChanges[inv]
	if !ok {
		return 0, fmt.Errorf("slot pointed to stack request %v with container %v, but that container was not changed in the request", slot.StackNetworkID, slot.Container.ContainerID)
	}
	actual, ok := changes[slot.Slot]
	if !ok {
		return 0, fmt.Errorf("slot pointed to stack request %v with container %v and slot %v, but that slot was not changed in the request", slot.StackNetworkID, slot.Container.ContainerID, slot.Slot)
	}
	return actual.id, nil
}

// tryAcknowledgeChanges iterates through all cached response changes and checks if the stack request slot
// info passed from the client has the right stack network ID in any of the stored slots. If this is the case,
// that entry is removed, so that the maps are cleaned up eventually.
func (h *itemStackRequestHandler) tryAcknowledgeChanges(s *ScreenManager, slot protocol.StackRequestSlotInfo) error {
	inv, ok := s.invByID(int32(slot.Container.ContainerID))
	if !ok {
		return fmt.Errorf("could not find container with id %v", slot.Container.ContainerID)
	}

	for requestID, containerChanges := range h.responseChanges {
		for newInv, changes := range containerChanges {
			for slotIndex, val := range changes {
				if (slot.Slot == slotIndex && slot.StackNetworkID >= 0 && newInv == inv) || h.current.Sub(val.timestamp) > time.Second*5 {
					delete(changes, slotIndex)
				}
			}
			if len(changes) == 0 {
				delete(containerChanges, newInv)
			}
		}
		if len(containerChanges) == 0 {
			delete(h.responseChanges, requestID)
		}
	}
	return nil
}

// itemInSlot looks for the item in the slot as indicated by the slot info passed.
func (h *itemStackRequestHandler) itemInSlot(slot protocol.StackRequestSlotInfo, s *ScreenManager) (item.Stack, error) {
	inv, ok := s.invByID(int32(slot.Container.ContainerID))
	if !ok {
		return item.Stack{}, fmt.Errorf("unable to find container with ID %v", slot.Container.ContainerID)
	}

	sl := int(slot.Slot)
	if inv == s.OffHand {
		sl = 0
	}

	i, err := inv.Item(sl)
	if err != nil {
		return i, err
	}
	return i, nil
}

// setItemInSlot sets an item stack in the slot of a container present in the slot info.
func (h *itemStackRequestHandler) setItemInSlot(slot protocol.StackRequestSlotInfo, i item.Stack, s *ScreenManager) {
	inv, _ := s.invByID(int32(slot.Container.ContainerID))

	sl := int(slot.Slot)
	if inv == s.OffHand {
		sl = 0
	}

	before, _ := inv.Item(sl)
	_ = inv.SetItem(sl, i)

	respSlot := protocol.StackResponseSlotInfo{
		Slot:                 slot.Slot,
		HotbarSlot:           slot.Slot,
		Count:                byte(i.Count()),
		StackNetworkID:       item_id(i),
		DurabilityCorrection: int32(i.MaxDurability() - i.Durability()),
	}

	if h.changes[slot.Container.ContainerID] == nil {
		h.changes[slot.Container.ContainerID] = map[byte]changeInfo{}
	}
	h.changes[slot.Container.ContainerID][slot.Slot] = changeInfo{
		after:  respSlot,
		before: before,
	}

	if h.responseChanges[h.currentRequest] == nil {
		h.responseChanges[h.currentRequest] = map[*inventory.Inventory]map[byte]responseChange{}
	}
	if h.responseChanges[h.currentRequest][inv] == nil {
		h.responseChanges[h.currentRequest][inv] = map[byte]responseChange{}
	}
	h.responseChanges[h.currentRequest][inv][slot.Slot] = responseChange{
		id:        respSlot.StackNetworkID,
		timestamp: h.current,
	}
}

// reject rejects the item stack request sent by the client so that it is reverted client-side.
func (h *itemStackRequestHandler) reject(id int32, s *ScreenManager) {
	// Revert changes that we already made for valid actions.
	for container, slots := range h.changes {
		for slot, info := range slots {
			inv, _ := s.invByID(int32(container))
			_ = inv.SetItem(int(slot), info.before)
		}
	}

	h.changes = map[byte]map[byte]changeInfo{}
	h.pendingResults = nil
}

// call uses an event.Context, slot and item.Stack to call the event handler function passed. An error is returned if
// the event.Context was cancelled either before or after the call.
func call(ctx *inventory.Context, slot int, it item.Stack, f func(ctx *inventory.Context, slot int, it item.Stack)) error {
	if ctx.Cancelled() {
		return fmt.Errorf("action was cancelled")
	}
	f(ctx, slot, it)
	if ctx.Cancelled() {
		return fmt.Errorf("action was cancelled")
	}
	return nil
}

// noinspection ALL
//
//go:linkname item_id github.com/df-mc/dragonfly/server/item.id
func item_id(s item.Stack) int32

// noinspection ALL
//
//go:linkname world_add github.com/df-mc/dragonfly/server/world.add
func world_add(e world.Entity, w *world.World)
