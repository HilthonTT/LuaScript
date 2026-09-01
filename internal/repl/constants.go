package repl

const Logo = `
██╗     ██╗   ██╗ █████╗ ███████╗ ██████╗██████╗ ██╗██████╗ ████████╗
██║     ██║   ██║██╔══██╗██╔════╝██╔════╝██╔══██╗██║██╔══██╗╚══██╔══╝
██║     ██║   ██║███████║███████╗██║     ██████╔╝██║██████╔╝   ██║   
██║     ██║   ██║██╔══██║╚════██║██║     ██╔══██╗██║██╔═══╝    ██║   
███████╗╚██████╔╝██║  ██║███████║╚██████╗██║  ██║██║██║        ██║   
╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝ ╚═════╝╚═╝  ╚═╝╚═╝╚═╝        ╚═╝
`

const Oops = `
 ██████╗  ██████╗ ██████╗ ███████╗
██╔═══██╗██╔═══██╗██╔══██╗██╔════╝
██║   ██║██║   ██║██████╔╝███████╗
██║   ██║██║   ██║██╔═══╝ ╚════██║
╚██████╔╝╚██████╔╝██║     ███████║
 ╚═════╝  ╚═════╝ ╚═╝     ╚══════╝`

const (
	promptReady = "\033[35mluascript » \033[0m"
	contPrompt  = "\033[31m   … \033[0m"

	colorErr   = "\033[31m"
	colorOK    = "\033[36m"
	colorBold  = "\033[1m"
	colorDim   = "\033[2m"
	colorReset = "\033[0m"

	cmdExit  = "exit"
	cmdQuit  = "quit"
	cmdReset = "reset"
	cmdClear = "clear"
	cmdHelp  = "help"
	cmdDoc   = "doc"
)

var luaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end", "false", "for",
	"function", "goto", "if", "in", "local", "nil", "not", "or",
	"repeat", "return", "then", "true", "until", "while",
	cmdHelp, cmdReset, cmdExit, cmdQuit, cmdDoc,
}
