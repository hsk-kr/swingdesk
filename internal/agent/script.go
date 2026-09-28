package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hsk-kr/swingdesk/internal/config"
	"github.com/hsk-kr/swingdesk/internal/model"
)

// researchTools is the only tool set a job may use.
const researchTools = "WebSearch,WebFetch"

// ClaudeOptions are the CLI knobs taken from config.
type ClaudeOptions struct {
	Bin             string
	Model           string // empty = CLI default (currently Opus)
	PermissionMode  config.PermissionMode
	SkipPermissions bool    // explicit opt-in; replaces --permission-mode
	MaxBudgetUSD    float64 // 0 = no cap
}

// jobFiles are the per-job file names inside the run directory.
type jobFiles struct {
	prompt, script, out, tmp, stderr, code, exit string
}

func filesFor(job model.Job) jobFiles {
	j := string(job)
	return jobFiles{
		prompt: j + ".prompt.md",
		script: j + ".sh",
		out:    j + ".json",
		tmp:    j + ".json.tmp",
		stderr: j + ".stderr",
		code:   j + ".code",
		exit:   j + ".exit",
	}
}

// claudeArgs builds the argv tail after the prompt. The prompt and schema are
// read from files inside the script to avoid quoting multi-kilobyte strings.
func claudeArgs(o ClaudeOptions) []string {
	args := []string{
		"--output-format", "json",
		"--tools", researchTools,
		"--allowedTools", researchTools,
		"--safe-mode", // no user CLAUDE.md, plugins, hooks or MCP in research jobs
		"--strict-mcp-config",
		"--no-session-persistence",
	}
	if o.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	} else {
		args = append(args, "--permission-mode", string(o.PermissionMode))
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(o.MaxBudgetUSD, 'f', -1, 64))
	}
	return args
}

// buildScript returns a POSIX sh script that runs one job in its tmux pane.
// Claude's stdout goes to <job>.json.tmp and is renamed to <job>.json only on
// success; stderr is shown in the pane and kept in <job>.stderr; the exit
// code is published atomically as <job>.exit for the runner to poll. Ctrl-C
// in the attached pane publishes 130. HUP/TERM are deliberately not trapped:
// sh must die so the kernel hangs up claude's process group too; the runner
// notices the dead pane.
func buildScript(job model.Job, runID int64, runDir string, o ClaudeOptions) string {
	f := filesFor(job)
	quoted := make([]string, 0, 16)
	for _, a := range claudeArgs(o) {
		quoted = append(quoted, shellQuote(a))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#!/bin/sh\n# swingdesk %s job, run %d (generated; do not edit)\n", job, runID)
	fmt.Fprintf(&b, "cd %s || exit 90\n", shellQuote(runDir))
	fmt.Fprintf(&b, "publish() { printf '%%s\\n' \"$1\" > %s.tmp && mv %s.tmp %s; }\n", f.exit, f.exit, f.exit)
	b.WriteString("trap 'publish 130; exit 130' INT\n")
	fmt.Fprintf(&b, "printf '▶ swingdesk %s job · run %d · started %%s\\n' \"$(date '+%%H:%%M:%%S')\"\n", job, runID)
	b.WriteString("printf '  Claude is researching; the result lands in this directory when done.\\n\\n'\n")
	fmt.Fprintf(&b, "{ %s -p \"$(cat %s)\" --json-schema \"$(cat schema.json)\" %s 2>&1 1>&3 3>&-; echo \"$?\" > %s; } 3> %s | tee %s\n",
		shellQuote(o.Bin), f.prompt, strings.Join(quoted, " "), f.code, f.tmp, f.stderr)
	fmt.Fprintf(&b, "code=$(cat %s 2>/dev/null || echo 99)\n", f.code)
	fmt.Fprintf(&b, "if [ \"$code\" -eq 0 ]; then mv %s %s; fi\n", f.tmp, f.out)
	b.WriteString("trap - INT\npublish \"$code\"\n")
	b.WriteString("printf '\\n■ exit %s at %s\\n' \"$code\" \"$(date '+%H:%M:%S')\"\n")
	return b.String()
}

// shellQuote single-quotes s for POSIX sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
