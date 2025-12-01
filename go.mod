module aws-asg-status

go 1.24.0

require (
	github.com/aws/aws-sdk-go-v2 v1.40.0
	github.com/aws/aws-sdk-go-v2/credentials v1.18.17
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.10
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.59.4
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.275.0
	github.com/mattn/go-isatty v0.0.20
	github.com/sirupsen/logrus v1.9.3
)

require (
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.14 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.14 // indirect
	github.com/aws/smithy-go v1.23.2 // indirect
	golang.org/x/sys v0.37.0 // indirect
)
