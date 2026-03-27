// SlashStage 🎸 — Cross AI IDE/CLI Project Management Dashboard
// Every project is a stage.
//
// Usage:
//
//	slashstage              — Start the dashboard (localhost:3000)
//	slashstage scan         — Scan all IDEs and print results
//	slashstage projects     — List registered projects
//	slashstage sessions     — List all sessions
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	"github.com/kuk1song/slashstage/internal/db"
	"github.com/kuk1song/slashstage/internal/parser"
	"github.com/kuk1song/slashstage/internal/server"

	// Register all parsers
	_ "github.com/kuk1song/slashstage/internal/parser"
)

const version = "0.1.0"

func main() {
	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Parse simple subcommand
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("slashstage v%s\n", version)
		return
	case "scan":
		runScan()
		return
	case "projects":
		runProjects()
		return
	case "sessions":
		runSessions()
		return
	case "help", "--help", "-h":
		printUsage()
		return
	default:
		// Default: start the web server
		runServer()
	}
}

func runServer() {
	dbPath, err := db.DefaultDBPath()
	if err != nil {
		slog.Error("cannot determine database path", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		slog.Error("cannot open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	addr := ":3000"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	fmt.Println()
	fmt.Println("  🎸 SlashStage v" + version)
	fmt.Println()
	fmt.Printf("  Dashboard: http://localhost%s\n", addr)
	fmt.Printf("  Database:  %s\n", dbPath)
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()

	// Auto-open browser
	go openBrowser("http://localhost" + addr)

	srv := server.New(database, addr)
	if err := srv.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func runScan() {
	fmt.Println("🔍 Scanning all IDEs for sessions...")
	fmt.Println()

	results, err := parser.ParseAll()
	if err != nil {
		slog.Error("scan failed", "error", err)
		os.Exit(1)
	}

	// Group by agent type
	byAgent := make(map[string]int)
	for _, r := range results {
		byAgent[r.Session.AgentType]++
	}

	fmt.Printf("Found %d sessions:\n", len(results))
	for agent, count := range byAgent {
		fmt.Printf("  %-20s %d sessions\n", agent, count)
	}

	// Discover projects
	candidates := parser.DiscoverProjects(results)
	fmt.Printf("\nFound %d potential projects:\n", len(candidates))
	for _, c := range candidates {
		fmt.Printf("  %s\n", c.String())
	}
}

func runProjects() {
	dbPath, err := db.DefaultDBPath()
	if err != nil {
		slog.Error("cannot determine database path", "error", err)
		os.Exit(1)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		slog.Error("cannot open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	projects, err := database.ListProjects()
	if err != nil {
		slog.Error("cannot list projects", "error", err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Println("No projects registered. Run 'slashstage' to start the dashboard and scan.")
		return
	}

	fmt.Printf("%-5s %-20s %s\n", "ID", "Name", "Path")
	fmt.Println("─────────────────────────────────────────────────────")
	for _, p := range projects {
		fmt.Printf("%-5d %-20s %s\n", p.ID, p.Name, p.Path)
	}
}

func runSessions() {
	fmt.Println("🔍 Discovering sessions...")

	files, err := parser.DiscoverAll()
	if err != nil {
		slog.Error("discover failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("\nFound %d session files:\n", len(files))
	for _, f := range files {
		fmt.Printf("  [%-12s] %s\n", f.AgentType, f.Path)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Run()
}

func printUsage() {
	fmt.Println(`🎸 SlashStage — Cross AI IDE/CLI Project Management Dashboard

Usage:
  slashstage              Start the dashboard (default: http://localhost:3000)
  slashstage scan         Scan all IDEs and print discovered sessions
  slashstage projects     List registered projects
  slashstage sessions     List discovered session files
  slashstage version      Print version
  slashstage help         Show this help

Environment:
  PORT                    HTTP port (default: 3000)

Data:
  ~/.slashstage/data.db   SQLite database`)
}
