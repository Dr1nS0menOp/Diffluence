// This file keeps the frontend out of the Go module. Some npm packages ship Go
// code, and "go test ./..." would otherwise build what is in node_modules.
module github.com/diffluence/diffluence/web

go 1.26.0
