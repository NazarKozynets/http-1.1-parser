package http1

type Request struct {
    Method  string
    Path    string
    Protocol string
    Headers map[string]string
    Body    []byte
}