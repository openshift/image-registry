package server

import "fmt"

type imageStream struct {
	namespace string
	name      string
}

func (is *imageStream) Reference() string {
	return fmt.Sprintf("%s/%s", is.namespace, is.name)
}
