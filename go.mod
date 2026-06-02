module aws-asg-status

go 1.24.0

require (
	github.com/aws/aws-sdk-go-v2 v1.41.9
	github.com/aws/aws-sdk-go-v2/credentials v1.19.7
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.67.0
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.279.1
	github.com/mattn/go-isatty v0.0.20
	github.com/sirupsen/logrus v1.9.4
)

require (
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/smithy-go v1.26.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)
