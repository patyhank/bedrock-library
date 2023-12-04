package bot

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"golang.org/x/exp/maps"
	"sync"
)

type Positioner struct {
	// Position is the position to spawn the entity on. If the entity is on a distance that the player cannot
	// see it, the entity will still show up if the player moves closer.
	Position mgl32.Vec3
	// Pitch is the vertical rotation of the player. Facing straight forward yields a pitch of 0. Pitch is
	// measured in degrees.
	Pitch float32
	// Yaw is the horizontal rotation of the player. Yaw is also measured in degrees.
	Yaw float32
	// HeadYaw is the same as Yaw, except that it applies specifically to the head of the player. A different
	// value for HeadYaw than Yaw means that the player will have its head turned.
	HeadYaw float32
	// BodyYaw is the same as Yaw, except that it applies specifically to the body of the entity. A different value for
	// BodyYaw than HeadYaw means that the entity will have its body turned, although it is unclear what the difference
	// between BodyYaw and Yaw is.
	//BodyYaw float32
	OnGround bool
}

type Entity struct {
	*Positioner
	EntityUniqueID int64
	// EntityRuntimeID is the runtime ID of the entity. The runtime ID is unique for each world session, and
	// entities are generally identified in packets using this runtime ID.
	EntityRuntimeID uint64
	// EntityType is the string entity type of the entity, for example 'minecraft:skeleton'. A list of these
	// entities may be found online.
	EntityType string
	// Velocity is the initial velocity the entity spawns with. This velocity will initiate client side
	// movement of the entity.
	Velocity mgl32.Vec3
	// BodyYaw is the same as Yaw, except that it applies specifically to the body of the entity. A different value for
	// BodyYaw than HeadYaw means that the entity will have its body turned, although it is unclear what the difference
	// between BodyYaw and Yaw is.
	BodyYaw float32
	// Attributes is a slice of attributes that the entity has. It includes attributes such as its health,
	// movement speed, etc.
	Attributes []protocol.AttributeValue
	// EntityMetadata is a map of entity metadata, which includes flags and data properties that alter in
	// particular the way the entity looks. Flags include ones such as 'on fire' and 'sprinting'.
	// The metadata values are indexed by their property key.
	EntityMetadata map[uint32]any
	// EntityProperties is a list of properties that the entity inhibits. These properties define and alter specific
	// attributes of the entity.
	EntityProperties protocol.EntityProperties
	// EntityLinks is a list of entity links that are currently active on the entity. These links alter the
	// way the entity shows up when first spawned in terms of it shown as riding an entity. Setting these
	// links is important for new viewers to see the entity is riding another entity.
	EntityLinks []protocol.EntityLink
}

type Player struct {
	*Positioner
	// UUID is the UUID of the player. It is the same UUID that the client sent in the Login packet at the
	// start of the session. A player with this UUID must exist in the player list (built up using the
	// PlayerList packet), for it to show up in-game.
	UUID uuid.UUID
	// Username is the name of the player. This username is the username that will be set as the initial
	// name tag of the player.
	Username string
	// EntityRuntimeID is the runtime ID of the player. The runtime ID is unique for each world session, and
	// entities are generally identified in packets using this runtime ID.
	EntityRuntimeID uint64
	// PlatformChatID is an identifier only set for particular platforms when chatting (presumably only for
	// Nintendo Switch). It is otherwise an empty string, and is used to decide which players are able to
	// chat with each other.
	PlatformChatID string
	// Velocity is the initial velocity the player spawns with. This velocity will initiate client side
	// movement of the player.
	Velocity mgl32.Vec3

	// HeldItem is the item that the player is holding. The item is shown to the viewer as soon as the player
	// itself shows up. Needless to say that this field is rather pointless, as additional packets still must
	// be sent for Armour to show up.
	HeldItem protocol.ItemInstance
	// GameType is the game type of the player. If set to GameTypeSpectator, the player will not be shown to viewers.
	GameType int32
	// EntityMetadata is a map of entity metadata, which includes flags and data properties that alter in
	// particular the way the player looks. Flags include ones such as 'on fire' and 'sprinting'.
	// The metadata values are indexed by their property key.
	EntityMetadata map[uint32]any
	// EntityProperties is a list of properties that the entity inhibits. These properties define and alter specific
	// attributes of the entity.
	EntityProperties protocol.EntityProperties
	// AbilityData represents various data about the abilities of a player, such as ability layers or permissions.
	AbilityData protocol.AbilityData
	// EntityLinks is a list of entity links that are currently active on the player. These links alter the
	// way the player shows up when first spawned in terms of it shown as riding an entity. Setting these
	// links is important for new viewers to see the player is riding another entity.
	EntityLinks []protocol.EntityLink
	// DeviceID is the device ID set in one of the files found in the storage of the device of the player. It
	// may be changed freely, so it should not be relied on for anything.
	DeviceID string
	// BuildPlatform is the build platform/device OS of the player that is about to be added, as it sent in
	// the Login packet when joining.
	BuildPlatform int32
}

type ItemEntity struct {
	*Positioner
	// EntityUniqueID is the unique ID of the entity. The unique ID is a value that remains consistent across
	// different sessions of the same world, but most servers simply fill the runtime ID of the entity out for
	// this field.
	EntityUniqueID int64
	// EntityRuntimeID is the runtime ID of the entity. The runtime ID is unique for each world session, and
	// entities are generally identified in packets using this runtime ID.
	EntityRuntimeID uint64
	// Item is the item that is spawned. It must have a valid ID for it to show up client-side. If it is not
	// a valid item, the client will crash when coming near.
	Item protocol.ItemInstance
	// Velocity is the initial velocity the entity spawns with. This velocity will initiate client side
	// movement of the entity.
	Velocity mgl32.Vec3
	// EntityMetadata is a map of entity metadata, which includes flags and data properties that alter in
	// particular the way the entity looks. Flags include ones such as 'on fire' and 'sprinting'.
	// The metadata values are indexed by their property key.
	EntityMetadata map[uint32]any
	// FromFishing specifies if the item was obtained by fishing it up using a fishing rod. It is not clear
	// why the client needs to know this.
	FromFishing bool
}

type EntityManager struct {
	pMutex   sync.Mutex
	players  map[uint64]*Player
	eMutex   sync.Mutex
	entities map[uint64]*Entity
	items    map[uint64]*ItemEntity
	iMutex   sync.Mutex
}

func NewEntityManager() *EntityManager {
	return &EntityManager{
		players:  make(map[uint64]*Player),
		pMutex:   sync.Mutex{},
		entities: make(map[uint64]*Entity),
		eMutex:   sync.Mutex{},
		items:    make(map[uint64]*ItemEntity),
		iMutex:   sync.Mutex{},
	}
}
func (n *EntityManager) MoveEntity(pkData *packet.MoveActorAbsolute) {
	rID := pkData.EntityRuntimeID
	var ent *Positioner
	if entity := n.GetEntity(rID); entity != nil {
		n.eMutex.Lock()
		defer n.eMutex.Unlock()
		ent = entity.Positioner
	}
	if entity := n.GetItem(rID); entity != nil {
		n.iMutex.Lock()
		defer n.iMutex.Unlock()
		ent = entity.Positioner
	}
	if entity := n.GetPlayer(rID); entity != nil {
		n.pMutex.Lock()
		defer n.pMutex.Unlock()
		ent = entity.Positioner
	}
	if ent == nil {
		return
	}
	ent.Position = pkData.Position
	ent.Pitch = pkData.Position[0]
	ent.Yaw = pkData.Position[1]
	ent.HeadYaw = pkData.Position[2]
}
func (n *EntityManager) MovePlayer(pkData *packet.MovePlayer) {
	if entity := n.GetPlayer(pkData.EntityRuntimeID); entity != nil {
		n.pMutex.Lock()
		defer n.pMutex.Unlock()
		ent := entity.Positioner

		ent.Position = pkData.Position
		ent.Pitch = pkData.Position[0]
		ent.Yaw = pkData.Position[1]
		ent.HeadYaw = pkData.Position[2]
	}
}
func (n *EntityManager) RemoveEntity(rID int64) {
	if entity := n.GetEntity(uint64(rID)); entity != nil {
		n.eMutex.Lock()
		defer n.eMutex.Unlock()
		delete(n.entities, uint64(rID))
	}
	if entity := n.GetItem(uint64(rID)); entity != nil {
		n.iMutex.Lock()
		defer n.iMutex.Unlock()
		delete(n.items, uint64(rID))
	}
	if entity := n.GetPlayer(uint64(rID)); entity != nil {
		n.pMutex.Lock()
		defer n.pMutex.Unlock()
		delete(n.players, uint64(rID))
	}
}

func (n *EntityManager) AddEntity(actor *packet.AddActor) {
	n.eMutex.Lock()
	defer n.eMutex.Unlock()
	n.entities[actor.EntityRuntimeID] = &Entity{
		Positioner: &Positioner{
			Position: actor.Position,
			Pitch:    actor.Pitch,
			Yaw:      actor.Yaw,
			HeadYaw:  actor.HeadYaw,
			OnGround: true,
		},
		EntityUniqueID:   actor.EntityUniqueID,
		EntityRuntimeID:  actor.EntityRuntimeID,
		EntityType:       actor.EntityType,
		Velocity:         actor.Velocity,
		BodyYaw:          actor.BodyYaw,
		Attributes:       actor.Attributes,
		EntityMetadata:   actor.EntityMetadata,
		EntityProperties: actor.EntityProperties,
		EntityLinks:      actor.EntityLinks,
	}
}
func (n *EntityManager) AddItems(actor *packet.AddItemActor) {
	n.iMutex.Lock()
	defer n.iMutex.Unlock()
	n.items[actor.EntityRuntimeID] = &ItemEntity{
		Positioner: &Positioner{
			Position: actor.Position,
			OnGround: true,
		},
		EntityUniqueID:  actor.EntityUniqueID,
		EntityRuntimeID: actor.EntityRuntimeID,
		Item:            actor.Item,
		Velocity:        actor.Velocity,
		EntityMetadata:  actor.EntityMetadata,
		FromFishing:     actor.FromFishing,
	}
}
func (n *EntityManager) AddPlayer(actor *packet.AddPlayer) {
	n.pMutex.Lock()
	defer n.pMutex.Unlock()
	n.players[actor.EntityRuntimeID] = &Player{
		Positioner: &Positioner{
			Position: actor.Position,
			Pitch:    actor.Pitch,
			Yaw:      actor.Yaw,
			HeadYaw:  actor.HeadYaw,
			OnGround: true,
		},
		UUID:             actor.UUID,
		Username:         actor.Username,
		EntityRuntimeID:  actor.EntityRuntimeID,
		PlatformChatID:   actor.PlatformChatID,
		Velocity:         actor.Velocity,
		HeldItem:         actor.HeldItem,
		GameType:         actor.GameType,
		EntityMetadata:   actor.EntityMetadata,
		EntityProperties: actor.EntityProperties,
		AbilityData:      actor.AbilityData,
		EntityLinks:      actor.EntityLinks,
		DeviceID:         actor.DeviceID,
		BuildPlatform:    actor.BuildPlatform,
	}
}

func (n *EntityManager) MoveEntityDel(pkData *packet.MoveActorDelta) {
	rID := pkData.EntityRuntimeID
	var ent *Positioner
	if entity := n.GetEntity(rID); entity != nil {
		n.eMutex.Lock()
		defer n.eMutex.Unlock()
		ent = entity.Positioner
	}
	if entity := n.GetItem(rID); entity != nil {
		n.iMutex.Lock()
		defer n.iMutex.Unlock()
		ent = entity.Positioner
	}
	if entity := n.GetPlayer(rID); entity != nil {
		n.pMutex.Lock()
		defer n.pMutex.Unlock()
		ent = entity.Positioner
	}
	if ent == nil {
		return
	}
	if pkData.Flags&packet.MoveActorDeltaFlagHasX == pkData.Flags {
		ent.Position[0] = pkData.Position.X()
	}
	if pkData.Flags&packet.MoveActorDeltaFlagHasY == pkData.Flags {
		ent.Position[1] = pkData.Position.Y()
	}
	if pkData.Flags&packet.MoveActorDeltaFlagHasZ == pkData.Flags {
		ent.Position[2] = pkData.Position.Z()
	}
	if pkData.Flags&packet.MoveActorDeltaFlagHasRotX == pkData.Flags {
		ent.Pitch = pkData.Rotation.X()
	}
	if pkData.Flags&packet.MoveActorDeltaFlagHasRotY == pkData.Flags {
		ent.Yaw = pkData.Rotation.Y()
	}
	if pkData.Flags&packet.MoveActorDeltaFlagHasZ == pkData.Flags {
		ent.HeadYaw = pkData.Rotation.Z()
	}
}

func (n *EntityManager) GetPlayers() map[uint64]*Player {
	n.pMutex.Lock()
	data := maps.Clone(n.players)
	n.pMutex.Unlock()
	return data
}
func (n *EntityManager) GetEntities() map[uint64]*Entity {
	n.eMutex.Lock()
	data := maps.Clone(n.entities)
	n.eMutex.Unlock()
	return data
}
func (n *EntityManager) GetItems() map[uint64]*ItemEntity {
	n.iMutex.Lock()
	data := maps.Clone(n.items)
	n.iMutex.Unlock()
	return data
}
func (n *EntityManager) GetPlayer(rID uint64) *Player {
	n.pMutex.Lock()
	defer n.pMutex.Unlock()
	return n.players[rID]
}
func (n *EntityManager) GetEntity(rID uint64) *Entity {
	n.eMutex.Lock()
	defer n.eMutex.Unlock()
	return n.entities[rID]
}
func (n *EntityManager) GetItem(rID uint64) *ItemEntity {
	n.iMutex.Lock()
	defer n.iMutex.Unlock()
	return n.items[rID]
}
