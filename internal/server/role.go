package server

type BotRole string

const (
	BotRolePlayer BotRole = "player"
	BotRoleNPC    BotRole = "npc"
)

func normalizeRole(role string) BotRole {
	switch role {
	case string(BotRolePlayer):
		return BotRolePlayer
	case string(BotRoleNPC):
		return BotRoleNPC
	default:
		return BotRoleNPC
	}
}
