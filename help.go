// Package strategy_game exposes editable, embedded game documents.
package strategy_game

import _ "embed"

// HelpMarkdown is the single source of the in-game reference. Keeping the
// Markdown under docs makes it comfortable to update in an editor, while
// The following embed directive also ships the file inside the executable.
//
//go:embed docs/HELP.md
var HelpMarkdown string
