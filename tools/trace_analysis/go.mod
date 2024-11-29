module github.com/vhive-serverless/sampler/tools/trace_analysis

go 1.22.7

require (
	github.com/gocarina/gocsv v0.0.0-20240520201108-78e41c74b4b1
	github.com/sirupsen/logrus v1.9.3
	github.com/vhive-serverless/loader v0.0.0-00010101000000-000000000000
)

require (
	golang.org/x/exp v0.0.0-20241009180824-f66d83c29e7c // indirect
	golang.org/x/sys v0.27.0 // indirect
	gonum.org/v1/gonum v0.15.1 // indirect
)

replace github.com/vhive-serverless/loader => ../../
