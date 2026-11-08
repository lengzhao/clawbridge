package discord

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/lengzhao/agentkit"
	"github.com/lengzhao/agentkit/cap/ingress"
	"github.com/lengzhao/agentkit/runtime/platform/common"
)

type delivery struct {
	channelID string
}

func (p *Platform) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil || m.Author.Bot {
		return
	}
	p.mu.RLock()
	botID := p.botID
	p.mu.RUnlock()
	if m.Author.ID == botID {
		return
	}
	if m.Timestamp.Before(common.ProcessStartTime.Add(-2 * time.Second)) {
		return
	}
	if !common.AllowList(p.cfg.AllowFrom, m.Author.ID) {
		return
	}
	if p.dedup.IsDuplicate("discord:" + m.ID) {
		return
	}

	botRoleID := p.botRoleIDForGuild(m.GuildID)
	if botRoleID == "" && m.GuildID != "" && s != nil {
		p.cacheBotRoleIDForGuild(s, m.GuildID, nil)
		botRoleID = p.botRoleIDForGuild(m.GuildID)
	}
	if m.GuildID != "" && !p.isGroupReplyAllGuild(m.GuildID) {
		if !isDiscordBotMention(m, botID, botRoleID, p.cfg.RespondToAtEveryoneAndHere) {
			return
		}
		m.Content = stripDiscordMentionWithRole(m.Content, botID, botRoleID)
		if m.MentionEveryone {
			m.Content = stripEveryoneHere(m.Content)
		}
	}

	content := strings.TrimSpace(m.Content)
	if content == "" {
		return
	}

	userID := m.Author.ID
	userName := m.Author.Username
	deliveryID := buildSessionKey(m.ChannelID, userID, p.cfg.ShareSessionInChannel)
	p.routes.Store(deliveryID, delivery{channelID: m.ChannelID})

	kind := "direct"
	if m.GuildID != "" {
		kind = "group"
	}
	route := agentkit.SessionRouteInput{
		Platform:    "discord",
		Instance:    p.platformInstance(),
		DeliveryID:  deliveryID,
		ChannelID:   m.ChannelID,
		ScopeUserID: userID,
	}
	wsOpts := common.InboundOptsFor(p.deps.Workspace)
	event := common.InboundFromContent("", route, userID, content, "", nil, nil, nil, nil, wsOpts)
	meta := map[string]any{
		"sender_id": userID, "sender_name": userName, "chat_id": m.ChannelID,
		"platform": "discord", "message_id": m.ID, "chat_type": kind,
	}
	if m.GuildID != "" {
		meta["guild_id"] = m.GuildID
	}
	event = common.WithMetadata(event, meta)

	ctx := context.Background()
	if p.deps.InboundPolicy != nil {
		raw, _ := json.Marshal(m)
		in := ingress.Input{
			Platform: "discord", InstanceID: p.platformInstance(), Kind: ingress.Message,
			ID: m.ID, ChatID: m.ChannelID, UserID: userID, ConversationKind: kind,
			Raw: raw,
			Message: &ingress.MessageFacts{
				HasText:          true,
				AddressedToAgent: m.GuildID == "" || p.isGroupReplyAllGuild(m.GuildID) || isDiscordBotMention(m, botID, botRoleID, p.cfg.RespondToAtEveryoneAndHere),
			},
			Default: ingress.Plan{Agent: ingress.Submit, Handlers: true},
		}
		decision, err := p.deps.InboundPolicy.Evaluate(ctx, in)
		if err != nil {
			slog.Error("discord: inbound policy", "error", err)
			return
		}
		plan, err := ingress.Resolve(in, decision)
		if err != nil || plan.Rejected || plan.Agent == ingress.Skip {
			return
		}
	}
	if err := p.inbox.Push(ctx, event); err != nil {
		slog.Debug("discord: inbox push", "error", err)
	}
}

func (p *Platform) botRoleIDForGuild(guildID string) string {
	if guildID == "" {
		return ""
	}
	v, ok := p.botRoleIDs.Load(guildID)
	if !ok {
		return ""
	}
	roleID, _ := v.(string)
	return roleID
}

func (p *Platform) cacheBotRoleIDForGuild(s *discordgo.Session, guildID string, guildRoles []*discordgo.Role) {
	if s == nil || guildID == "" {
		return
	}
	p.mu.RLock()
	botID := p.botID
	p.mu.RUnlock()
	if botID == "" {
		return
	}
	roleID, err := resolveBotManagedRole(s, guildID, botID, guildRoles)
	if err != nil {
		slog.Debug("discord: bot role resolve", "guild", guildID, "error", err)
		return
	}
	if roleID != "" {
		p.botRoleIDs.Store(guildID, roleID)
	}
}

func resolveBotManagedRole(s *discordgo.Session, guildID, botID string, guildRoles []*discordgo.Role) (string, error) {
	member, err := s.GuildMember(guildID, botID)
	if err != nil {
		return "", err
	}
	if member == nil || len(member.Roles) == 0 {
		return "", nil
	}
	memberRoles := make(map[string]struct{}, len(member.Roles))
	for _, id := range member.Roles {
		memberRoles[id] = struct{}{}
	}
	roles := guildRoles
	if len(roles) == 0 {
		roles, err = s.GuildRoles(guildID)
		if err != nil {
			return "", err
		}
	}
	for _, role := range roles {
		if role == nil || !role.Managed {
			continue
		}
		if _, ok := memberRoles[role.ID]; ok {
			return role.ID, nil
		}
	}
	return "", nil
}
