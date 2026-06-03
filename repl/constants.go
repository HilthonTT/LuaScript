package repl

// Logo is the ASCII banner shown at REPL start and at the top of `help`.
// Tagline / control-key info is intentionally NOT embedded — Start() and
// printHelp() print their own context-appropriate footer so the messages
// don't conflict (Ctrl+C cancels input; Ctrl+D exits).
const Logo = `
███████╗ █████╗ ██╗  ██╗██╗   ██╗██████╗  █████╗
██╔════╝██╔══██╗██║ ██╔╝██║   ██║██╔══██╗██╔══██╗
███████╗███████║█████╔╝ ██║   ██║██████╔╝███████║
╚════██║██╔══██║██╔═██╗ ██║   ██║██╔══██╗██╔══██║
███████║██║  ██║██║  ██╗╚██████╔╝██║  ██║██║  ██║
╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝
`

const Oops = `
 ██████╗  ██████╗ ██████╗ ███████╗
██╔═══██╗██╔═══██╗██╔══██╗██╔════╝
██║   ██║██║   ██║██████╔╝███████╗
██║   ██║██║   ██║██╔═══╝ ╚════██║
╚██████╔╝╚██████╔╝██║     ███████║
 ╚═════╝  ╚═════╝ ╚═╝     ╚══════╝`

const (
	//.lsc-themed colors — modern Windows terminals (10+) handle these
	// natively via virtual-terminal processing; chzyer/readline turns that
	// on for us.
	promptReady = "\033[35m.lsc » \033[0m" // Magenta — classic.lsc feel
	contPrompt  = "\033[31m   … \033[0m"   // Red — visually distinct from ready prompt

	// Output accents. Keep these subtle so they don't compete with user output.
	colorErr   = "\033[31m" // red — error messages
	colorOK    = "\033[36m" // cyan — result prefix `=>`
	colorBold  = "\033[1m"  // emphasis in help text
	colorDim   = "\033[2m"  // de-emphasis (asides, reset confirmation)
	colorReset = "\033[0m"

	cmdExit  = "exit"
	cmdQuit  = "quit"
	cmdReset = "reset"
	cmdClear = "clear"
	cmdHelp  = "help"
)

var luaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
	cmdHelp, cmdReset, cmdExit, cmdQuit,
}
