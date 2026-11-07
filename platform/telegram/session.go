package telegram

import (
	"fmt"
	"strconv"

	"github.com/lengzhao/agentkit"
)

// buildSessionKey 与 cc-connect platform/telegram 一致，保证会话键可对照迁移。
func buildSessionKey(shareInChannel bool, chatID int64, threadID int, userID int64) agentkit.SessionID {
	if shareInChannel {
		if threadID != 0 {
			return agentkit.SessionID(fmt.Sprintf("telegram:%d:%d", chatID, threadID))
		}
		return agentkit.SessionID(fmt.Sprintf("telegram:%d", chatID))
	}
	if threadID != 0 {
		return agentkit.SessionID(fmt.Sprintf("telegram:%d:%d:%d", chatID, threadID, userID))
	}
	return agentkit.SessionID(fmt.Sprintf("telegram:%d:%d", chatID, userID))
}

func channelID(chatID int64, threadID int) string {
	if threadID != 0 {
		return fmt.Sprintf("%d:%d", chatID, threadID)
	}
	return strconv.FormatInt(chatID, 10)
}
