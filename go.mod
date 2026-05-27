module github.com/msnotfound/fleetorch

go 1.23

require (
	github.com/gofrs/flock v0.13.0
	github.com/spf13/cobra v1.10.2
)

replace github.com/gofrs/flock => ./internal/store/flock

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)
