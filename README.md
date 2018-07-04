[![Build Status](https://travis-ci.org/orisano/impast.svg?branch=master)](https://travis-ci.org/orisano/impast)
[![Maintainability](https://api.codeclimate.com/v1/badges/3cdd244b061420db53ba/maintainability)](https://codeclimate.com/github/orisano/impast/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/3cdd244b061420db53ba/test_coverage)](https://codeclimate.com/github/orisano/impast/test_coverage)

library for package AST importing.

## Installation
```sh
go get -u github.com/orisano/impast
```
## How to use
```go
package main

import (
	"fmt"
	"log"
	
	"github.com/orisano/impast"
)

func main() {
	pkg, err := impast.ImportPackage("io")
	if err != nil {
		log.Fatal(err)
	}
	it := impast.FindInterface(pkg, "Writer")
	if it == nil {
		log.Fatalf("io.Writer not found")
	}

	methods := impast.GetRequires(it)
	for _, method := range methods {
		fmt.Println(method.Names[0].Name)
	}
	// Output:
	// Write
}
```

## Useful commands
### interfacer
struct to interface command
#### Installation
```bash
go get -u github.com/orisano/impast/cmd/interfacer
```
#### How to use
```bash
$ interfacer -pkg net/http -type "*Client" -out HTTPClient
type HTTPClient interface {
	Get(url string) (resp *http.Response, err error)
	Do(req *http.Request) (*http.Response, error)
	Post(url string, contentType string, body io.Reader) (resp *http.Response, err error)
	PostForm(url string, data url.Values) (resp *http.Response, err error)
	Head(url string) (resp *http.Response, err error)
}
```

### mocker
generate mock command
#### Installation
```bash
go get -u github.com/orisano/impast/cmd/mocker
```
#### How to use
```bash
$ mocker -pkg io -type ReadWriter
type ReadWriterMock struct {
	ReadMock	func(p []byte) (n int, err error)
	WriteMock	func(p []byte) (n int, err error)
}

func (m *ReadWriterMock) Read(p []byte) (n int, err error) {
	return m.ReadMock(p)
}

func (m *ReadWriterMock) Write(p []byte) (n int, err error) {
	return m.WriteMock(p)
}
```

### stuber
generate stub command
#### Installation
```bash
go get -u github.com/orisano/impast/cmd/stuber
```
#### How to use
```bash
$ stuber -pkg net -implement Conn -export -name c -type "*MyConn"
func (c *MyConn) Read(b []byte) (n int, err error) {
	panic("implement me")
}

func (c *MyConn) Write(b []byte) (n int, err error) {
	panic("implement me")
}

func (c *MyConn) Close() error {
	panic("implement me")
}

func (c *MyConn) LocalAddr() net.Addr {
	panic("implement me")
}

func (c *MyConn) RemoteAddr() net.Addr {
	panic("implement me")
}

func (c *MyConn) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (c *MyConn) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (c *MyConn) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}
```

## License
MIT
