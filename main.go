// Encoding: UTF-8

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func init() {
	if _, ok := os.LookupEnv("AWS_ASG_STATUS_DEBUG"); ok {
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	}
}

func main() {
	flag.Parse()

	if versionFlag {
		showVersion()
		os.Exit(0)
	}

	if len(os.Args) == 1 {
		log.Fatal("Must supply an argument: enter-standby|exit-standby|healthy|unhealthy|status")
	}

	// AWS Session
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config:            *aws.NewConfig().WithCredentialsChainVerboseErrors(true),
		SharedConfigState: session.SharedConfigDisable,
	}))

	metadata := ec2metadata.New(sess)

	awsCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !metadata.AvailableWithContext(awsCtx) {
		log.Fatal("EC2 Metadata is not available... Are we running on an EC2 instance?")
	}

	identity, err := metadata.GetInstanceIdentityDocument()
	if err != nil {
		log.Fatal(err)
	}
	instanceID := identity.InstanceID
	sess.Config = sess.Config.WithRegion(identity.Region)

	ec2client := ec2.New(sess)

	input := &ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("resource-id"),
				Values: []*string{
					aws.String(instanceID),
				},
			},
		},
	}

	resp, err := ec2client.DescribeTags(input)
	if err != nil {
		log.Fatal(err)
	}

	tags := resp.Tags

	// Handle EC2 API Pagination
	for {
		if resp.NextToken == nil {
			break
		}

		input.NextToken = resp.NextToken

		resp, err := ec2client.DescribeTags(input)
		if err != nil {
			log.Fatal(err)
		}

		tags = append(tags, resp.Tags...)
	}

	var AsgName *string

	for _, tag := range tags {
		if *tag.Key == "aws:autoscaling:groupName" {
			AsgName = tag.Value
		}
	}

	if AsgName == nil {
		log.Fatal("Required tag: aws:autoscaling:groupName was not present on EC2 Instance!")
	}

	asClient := autoscaling.New(sess)

	switch os.Args[1] {
	case "enter-standby":
		standbyOut, err := asClient.EnterStandby(&autoscaling.EnterStandbyInput{
			AutoScalingGroupName:           AsgName,
			InstanceIds:                    []*string{aws.String(instanceID)},
			ShouldDecrementDesiredCapacity: aws.Bool(true),
		})
		if err != nil {
			log.Fatal(err)
		}
		prettyPrint(standbyOut)
	case "exit-standby":
		activeOut, err := asClient.ExitStandby(&autoscaling.ExitStandbyInput{
			AutoScalingGroupName: AsgName,
			InstanceIds:          []*string{aws.String(instanceID)},
		})
		if err != nil {
			log.Fatal(err)
		}
		prettyPrint(activeOut)
	case "healthy":
		_, err := asClient.SetInstanceHealth(&autoscaling.SetInstanceHealthInput{
			HealthStatus:             aws.String("Healthy"),
			InstanceId:               aws.String(instanceID),
			ShouldRespectGracePeriod: aws.Bool(false),
		})
		if err != nil {
			log.Fatal(err)
		}
	case "unhealthy":
		_, err := asClient.SetInstanceHealth(&autoscaling.SetInstanceHealthInput{
			HealthStatus:             aws.String("Unhealthy"),
			InstanceId:               aws.String(instanceID),
			ShouldRespectGracePeriod: aws.Bool(false),
		})
		if err != nil {
			log.Fatal(err)
		}
	case "status":
		describeAsgsOut, err := asClient.DescribeAutoScalingGroups(&autoscaling.DescribeAutoScalingGroupsInput{
			AutoScalingGroupNames: []*string{AsgName},
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Println(describeAsgsOut)
	default:
		log.Fatalln("Unknown argument:", os.Args[1])
	}
}

func prettyPrint(obj interface{}) {
	prettyJSON, err := json.MarshalIndent(obj, "", "    ")
	if err != nil {
		log.Fatalln("Failed to Marshal JSON:", err)
	}
	fmt.Println(string(prettyJSON))
}
