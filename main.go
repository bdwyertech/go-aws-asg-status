// Encoding: UTF-8

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/mattn/go-isatty"
)

var healthcheckUrl string
var healthcheckTimeout time.Duration

func init() {
	flag.StringVar(&healthcheckUrl, "healthcheck-url", "", "Healthcheck endpoint URL")
	flag.DurationVar(&healthcheckTimeout, "healthcheck-timeout", 5*time.Minute, "Healthcheck timeout")

	if _, ok := os.LookupEnv("AWS_ASG_STATUS_DEBUG"); ok {
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	}
	// Workaround for https://github.com/PowerShell/PowerShell/issues/14273
	// PSNotApplyErrorActionToStderr
	if runtime.GOOS == "windows" && !(isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) {
		log.SetOutput(os.Stdout)
	}
}

func main() {
	flag.Parse()

	if versionFlag {
		showVersion()
		os.Exit(0)
	}

	if len(os.Args) == 1 && healthcheckUrl == "" {
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
			{
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

	// Wait for Healthcheck if configured

	switch os.Args[1] {
	case "healthy", "":
		status := aws.String("Healthy")
		var err error
		if healthcheckUrl != "" {
			if err = waitUntilHealthy(); err != nil {
				log.Error(err)
				status = aws.String("Unhealthy")
			}
		}
		if _, err = asClient.SetInstanceHealth(&autoscaling.SetInstanceHealthInput{
			HealthStatus:             status,
			InstanceId:               aws.String(instanceID),
			ShouldRespectGracePeriod: aws.Bool(false),
		}); err != nil {
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

func prettyPrint(obj any) {
	prettyJSON, err := json.MarshalIndent(obj, "", "    ")
	if err != nil {
		log.Fatalln("Failed to Marshal JSON:", err)
	}
	fmt.Println(string(prettyJSON))
}

func waitUntilHealthy() error {
	// Copy of http.DefaultTransport with Flippable TLS Verification
	// https://golang.org/pkg/net/http/#Client
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: func() bool {
				_, ok := os.LookupEnv("CFN_SIGNAL_SSL_VERIFY")
				return ok
			}()},
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	var bodyBytes []byte

	for {
		req, err := http.NewRequestWithContext(ctx, "GET", healthcheckUrl, nil)
		if err != nil {
			log.Fatal(err)
		}
		requestTimeout := 30 * time.Second
		rctx, rcancel := context.WithTimeout(ctx, requestTimeout)
		defer rcancel()
		resp, err := client.Do(req.WithContext(rctx))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr == context.DeadlineExceeded {
				if len(bodyBytes) > 0 {
					var prettyJSON bytes.Buffer
					if err := json.Indent(&prettyJSON, bodyBytes, "", "  "); err != nil {
						log.Error(string(bodyBytes))
					} else {
						log.Error(prettyJSON.String())
					}
				}
				return fmt.Errorf("healthcheck exceeded timeout(%s): %w", healthcheckTimeout, err)
			}
			if ctxErr := rctx.Err(); ctxErr == context.DeadlineExceeded {
				log.Warn(fmt.Errorf("healthcheck request timeout(%s): %w", requestTimeout, err))
			} else {
				log.Error(err)
			}
			time.Sleep(5 * time.Second)
			continue
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case 200:
			return nil
		default:
			log.Warnf("%v :: (%v) %v", healthcheckUrl, resp.StatusCode, resp.Status)
			bodyBytes, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			time.Sleep(5 * time.Second)
			continue
		}
	}
}
