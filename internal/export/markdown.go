// Package export provides session export functionality for SlashStage.
// The primary export format is Markdown, designed for context injection
// into other LLM agents when switching IDEs.
package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/kuk1song/slashstage/internal/model"
)

// ToMarkdown converts a session and its messages into a Markdown string.
// The output is designed to be placed in a project root and read by
// a new coding agent for seamless context continuation.
func ToMarkdown(session model.Session, messages []model.Message) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Session: %s\n\n", session.Title))
	sb.WriteString("## Metadata\n\n")
	sb.WriteString(fmt.Sprintf("- **ID**: `%s`\n", session.ID))
	sb.WriteString(fmt.Sprintf("- **Agent Type**: `%s`\n", session.AgentType))
	if session.Workspace != "" {
		sb.WriteString(fmt.Sprintf("- **Workspace**: `%s`\n", session.Workspace))
	}
	if session.Model != "" {
		sb.WriteString(fmt.Sprintf("- **Model**: `%s`\n", session.Model))
	}
	sb.WriteString(fmt.Sprintf("- **Started At**: %s\n", formatTime(session.StartedAt)))
	sb.WriteString(fmt.Sprintf("- **Last Active At**: %s\n", formatTime(session.LastActiveAt)))
	sb.WriteString(fmt.Sprintf("- **Source Path**: `%s`\n", session.SourcePath))
	sb.WriteString(fmt.Sprintf("- **Message Count**: %d\n", len(messages)))
	if session.TokensIn > 0 || session.TokensOut > 0 {
		sb.WriteString(fmt.Sprintf("- **Tokens In**: %d\n", session.TokensIn))
		sb.WriteString(fmt.Sprintf("- **Tokens Out**: %d\n", session.TokensOut))
	}
	sb.WriteString("\n---\n\n")

	if len(messages) == 0 {
		return sb.String()
	}

	sb.WriteString("## Conversation History\n\n")
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("### %s (%s)\n\n", formatRole(msg.Role), formatTime(msg.CreatedAt)))
		if msg.ToolName != "" {
			sb.WriteString(fmt.Sprintf("#### Tool Call: `%s`\n\n", msg.ToolName))
			if msg.ToolInput != "" {
				sb.WriteString("```json\n")
				sb.WriteString(msg.ToolInput)
				sb.WriteString("\n```\n\n")
			}
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.Format(time.RFC3339)
}

func formatRole(role model.MessageRole) string {
	switch role {
	case model.RoleUser:
		return "User"
	case model.RoleAssistant:
		return "Assistant"
	case model.RoleTool:
		return "Tool"
	case model.RoleSystem:
		return "System"
	default:
		return string(role)
	}
}

// Truncate a string to a maximum length, adding "..." if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
