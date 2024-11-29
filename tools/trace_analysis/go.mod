module github.com/vhive-serverless/sampler/tools/trace_analysis

go 1.26.2

require (
	github.com/gocarina/gocsv v0.0.0-20240520201108-78e41c74b4b1
	github.com/sirupsen/logrus v1.9.4
	github.com/vhive-serverless/loader v0.0.0-00010101000000-000000000000
)

require (
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/sys v0.43.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/vhive-serverless/loader => ../../
