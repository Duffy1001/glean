package extract

import "io"

type Source struct {
	Name   string
	Reader io.Reader
}
