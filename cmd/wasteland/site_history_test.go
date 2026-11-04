package main

import (
	"bytes"
	"testing"
)

func TestVisitRequestRejectsExternalAndMalformedPaths(t *testing.T) {
	for _, path := range []string{"https://evil.example/", "//evil.example/x", "", "/" + string(bytes.Repeat([]byte{'a'}, 513))} {
		body := visitRequest{Kind: "course", ID: "CS1", Path: path, Title: "Course"}
		if err := validateVisitRequest(&body); err == nil {
			t.Errorf("path %q should be rejected", path)
		}
	}
}

func TestVisitRequestRejectsUnknownKind(t *testing.T) {
	body := visitRequest{Kind: "admin", ID: "1", Path: "/admin.html", Title: "Admin"}
	if err := validateVisitRequest(&body); err == nil {
		t.Fatal("unknown kind should be rejected")
	}
}
