package box

import (
	"fmt"
)

type ErrConnect struct {
	Err error // Original error
}

func (e *ErrConnect) Error() string {
	return fmt.Sprintf("error connecting to process: %v", e.Err)
}
