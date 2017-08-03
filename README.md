argot
=====

A lightweight simple testing framework in Go. Comes with reasonable support for testing HTTP servers.

GoDoc available here: http://godoc.org/github.com/msackman/argot

Blog post available here: https://tech.labs.oliverwyman.com/blog/2017/04/21/argot-a-lightweight-composable-test-framework-for-go/

Build
=====

`go get github.com/msackman/argot`

Example
=======

Just to show off one example again:

    package main

    import (
        "errors"
        "github.com/msackman/argot"
        "net/http"
        "testing"
    )

    func AlwaysFails() argot.Step {
        return argot.NewNamedStep(
            "AlwaysSucceeds",
            func() error {
                return errors.New("nope")
            },
        )
    }

    func Testify(t *testing.T) {
        req := argot.NewHttpCall(nil)
        defer req.Reset()
        argot.Steps([]argot.Step{
            req.NewRequest("POST", server+"/api/v1/foo/bar", nil),
            req.RequestHeader("Content-Type", "application/json"),
            req.ResponseStatusEquals(http.StatusOK),
            req.ResponseHeaderEquals("Content-Type", "application/json"),
            req.ResponseHeaderNotExists("Magical-Response"),
            req.ResponseHeaderNotExists("No-Unicorns"),
            req.ResponseBodyEquals("Attack ships on fire off the shoulder of Orion...\n"),
            AlwaysFails(req),
        }).Test(t)
    }

As a result running `go test` will output a helpful log in case any of
the steps failed:

    % go test
    --- FAIL: Testify (0.24s)
            test.go:20: Achieved Steps:
                   [NewRequest(POST: https://...)
                    RequestHeader(Content-Type: application/json)
                    ResponseStatusEquals(200)
                    ResponseHeaderEquals(Content-Type: application/json)
                    ResponseHeaderNotExists(Magical-Response)
                    ResponseHeaderNotExists(No-Unicorns)]
                   Error: nope
    FAIL
