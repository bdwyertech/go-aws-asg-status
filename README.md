# AWS ASG Status

[![Build Status](https://github.com/bdwyertech/go-aws-asg-status/workflows/Go/badge.svg?branch=master)](https://github.com/bdwyertech/go-aws-asg-status/actions?query=workflow%3AGo+branch%3Amaster)
[![](https://images.microbadger.com/badges/image/bdwyertech/aws-asg-status.svg)](https://microbadger.com/images/bdwyertech/aws-asg-status)
[![](https://images.microbadger.com/badges/version/bdwyertech/aws-asg-status.svg)](https://microbadger.com/images/bdwyertech/aws-asg-status)

This is a tool to update an instances [status within an ASG.](https://docs.aws.amazon.com/autoscaling/ec2/APIReference/API_EnterStandby.html)

### Supported Arguments
* `enter-standby`
* `exit-standby`
* `status`

### Sample IAM Policy
This policy is locked down to scope IAM permissions to instances within its own ASG.  It leverages the built-in tags created by AWS CloudFormation.
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
            	"autoscaling:Describe*",
            	"ec2:DescribeTags"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
            	"autoscaling:EnterStandby",
            	"autoscaling:ExitStandby"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "autoscaling:ResourceTag/aws:cloudformation:stack-id": "${aws:ResourceTag/aws:cloudformation:stack-id}",
                    "autoscaling:ResourceTag/aws:cloudformation:logical-id": "${aws:ResourceTag/aws:cloudformation:logical-id}"
                }
            }
        }
    ]
}
```

Unfortunately, you cannot use the AWS ASG-defined tags in conditional access policies.

For example, this condition does not work:
```
"autoscaling:ResourceTag/aws:autoscaling:groupName": "${aws:ResourceTag/aws:autoscaling:groupName}"
```
