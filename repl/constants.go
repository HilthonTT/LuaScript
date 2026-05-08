package repl

const Logo = `
███████╗ █████╗ ██╗  ██╗██╗   ██╗██████╗  █████╗ 
██╔════╝██╔══██╗██║ ██╔╝██║   ██║██╔══██╗██╔══██╗
███████╗███████║█████╔╝ ██║   ██║██████╔╝███████║
╚════██║██╔══██║██╔═██╗ ██║   ██║██╔══██╗██╔══██║
███████║██║  ██║██║  ██╗╚██████╔╝██║  ██║██║  ██║
╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝

  A lua-based language with a stack-based VM.
  Type 'help' for commands, CTRL+C to exit
`

const Oops = `
 ██████╗  ██████╗ ██████╗ ███████╗
██╔═══██╗██╔═══██╗██╔══██╗██╔════╝
██║   ██║██║   ██║██████╔╝███████╗
██║   ██║██║   ██║██╔═══╝ ╚════██║
╚██████╔╝╚██████╔╝██║     ███████║
 ╚═════╝  ╚═════╝ ╚═╝     ╚══════╝`

const (
	// Sakura-themed colors - safer for Windows
	promptReady = "\033[35m sakura » \033[0m" // Magenta/Purple (classic sakura feel)
	contPrompt  = "\033[31m   … \033[0m"      // Red for continuation

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
