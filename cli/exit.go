package cli

const (
	exitLaunched  = 0
	exitNoMatch   = 1
	exitAmbiguous = 2
	exitLaunch    = 3
	exitError     = 4
)

// ExitOther is the exit code of an error that is no ExitError.
const ExitOther = exitError

// ExitError carries the process exit code. main unpacks it.
type ExitError struct {
	code int
	msg  string
}

func (e *ExitError) Error() string { return e.msg }

func (e *ExitError) Code() int { return e.code }
