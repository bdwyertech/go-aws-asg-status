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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

	imdsClient := imds.New(imds.Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	idDoc, err := imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
	if err != nil {
		log.Fatalf("failed to get instance identity document: %v", err)
	}
	instanceID := idDoc.InstanceID

	cfg := aws.Config{
		Region: idDoc.Region,
		// We should only ever be using this on EC2 Instances with an Instance Role...
		Credentials: ec2rolecreds.New(),
	}

	var tags []ec2types.TagDescription
	ec2Client := ec2.NewFromConfig(cfg)
	paginator := ec2.NewDescribeTagsPaginator(ec2Client, &ec2.DescribeTagsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("resource-id"),
				Values: []string{instanceID},
			},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("failed to describe EC2 tags: %v", err)
		}
		tags = append(tags, page.Tags...)
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

	asgClient := autoscaling.NewFromConfig(cfg)

	// Wait for Healthcheck if configured

	switch os.Args[1] {
	case "healthy", "":
		status := "Healthy"
		var err error
		if healthcheckUrl != "" {
			if err = waitUntilHealthy(); err != nil {
				log.Error(err)
				status = "Unhealthy"
			}
		}
		_, err = asgClient.SetInstanceHealth(ctx, &autoscaling.SetInstanceHealthInput{
			HealthStatus:             &status,
			InstanceId:               &instanceID,
			ShouldRespectGracePeriod: aws.Bool(false),
		})
		if err != nil {
			log.Fatal(err)
		}
	case "unhealthy":
		_, err := asgClient.SetInstanceHealth(ctx, &autoscaling.SetInstanceHealthInput{
			HealthStatus:             aws.String("Unhealthy"),
			InstanceId:               &instanceID,
			ShouldRespectGracePeriod: aws.Bool(false),
		})
		if err != nil {
			log.Fatal(err)
		}
	case "enter-standby":
		standbyOut, err := asgClient.EnterStandby(ctx, &autoscaling.EnterStandbyInput{
			AutoScalingGroupName:           AsgName,
			InstanceIds:                    []string{instanceID},
			ShouldDecrementDesiredCapacity: aws.Bool(true),
		})
		if err != nil {
			log.Fatal(err)
		}
		prettyPrint(standbyOut)
	case "exit-standby":
		activeOut, err := asgClient.ExitStandby(ctx, &autoscaling.ExitStandbyInput{
			AutoScalingGroupName: AsgName,
			InstanceIds:          []string{instanceID},
		})
		if err != nil {
			log.Fatal(err)
		}
		prettyPrint(activeOut)
	case "status":
		describeAsgsOut, err := asgClient.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
			AutoScalingGroupNames: []string{*AsgName},
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
