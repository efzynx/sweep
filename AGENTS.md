# AGENTS.md - Sweep

## Project Overview
Sweep is a modern, modular, cross-distribution Linux system cleaner and maintenance utility built with Go and Bubbletea (TUI first, native GUI planned for later phases).

## Primary Directory
`/home/efzyn/Project/sweep`

## Tech Stack
- Language: Go (v1.23+)
- TUI Framework: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`
- CLI Parsing: `github.com/spf13/cobra` (or stdlib `flag` for lean design)
- Target Platforms: All Linux distributions (Arch/CachyOS, Debian/Ubuntu, Fedora/RHEL, OpenSUSE, etc.)

## Architecture Guidelines
- Modular Adapter Architecture:
  * Maintain clean separation between the scanner/cleaner engine and presentation layer.
  * Every subsystem implements a common `Module` interface (`ID`, `Name`, `IsAvailable`, `Scan`, `Clean`).
  * No hardcoded distro logic in the core; use runtime environment detection adapters.
- Safety First:
  * Read-only scanning without root/sudo privilege wherever possible.
  * Explicit Dry-run support across all modules.
  * Elevate privileges only when executing clean actions that strictly require root.
  * Support optional Btrfs/Snapper safeguard snapshot prior to destructive actions.

## Repository Conventions
- Git default branch: `main`.
- Clean commit messages following conventional commits (`feat:`, `fix:`, `refactor:`, `docs:`).
- Always verify builds with `go build ./...` and `go test ./...` before reporting completion.
