package intern

import (
	"fmt"
    "encoding/json"
    "strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

    "github.com/jmoiron/jsonq"
)

type PublicBuckets struct {
    ACL    []string
    Policy []string
}

func GetBuckets(config aws.Config) ([]string, error) {
	svc := s3.New(session.New(), &config)
	input := &s3.ListBucketsInput{}

	result, err := svc.ListBuckets(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
		return []string{}, err
	}

	var buckets []string

	for bucket := range result.Buckets {
		buckets = append(buckets, aws.StringValue(result.Buckets[bucket].Name))
	}

	return buckets, nil
}

func OpenPolicy(policy *string) (bool) {
    data := map[string]interface{}{}
    dec := json.NewDecoder(strings.NewReader(aws.StringValue(policy)))
    dec.Decode(&data)
    jq := jsonq.NewQuery(data)

    effect, _ := jq.String("Statement", "0", "Effect")
    principal, _ := jq.String("Statement", "0", "Principal")
    action, _ := jq.String("Statement", "0", "Action")

    if effect == "Allow" && principal == "*" && action == "s3:GetObject" {
        return true
    }

    return false
}

func OpenACL(grants []*s3.Grant) (bool) {
    for grant := range grants {
        //fmt.Println(aws.StringValue(grants[grant].Grantee.URI))
        //fmt.Println(aws.StringValue(grants[grant].Permission))

        if aws.StringValue(grants[grant].Grantee.URI) == "http://acs.amazonaws.com/groups/global/AllUsers" {
            if aws.StringValue(grants[grant].Permission) == "READ" || aws.StringValue(grants[grant].Permission) == "FULL_CONTROL" || aws.StringValue(grants[grant].Permission) == "WRITE" {
                return true
            }
        }
    }

    return false
}

func GetBucketRegion(config aws.Config, bucket string) string {
    svc := s3.New(session.New(), &config)
    input := &s3.GetBucketLocationInput{}
    input.Bucket = aws.String(bucket)

    result, err := svc.GetBucketLocation(input)
    if err != nil {
        if aerr, ok := err.(awserr.Error); ok {
            switch aerr.Code() {
            default:
                fmt.Println(aerr.Error())
            }
        } else {
            // Print the error, cast err to awserr.Error to get the Code and
            // Message from an error.
            fmt.Println(err.Error())
        }
    }

    var region string

    if strings.Contains(result.String(), "LocationConstraint") {
        region = aws.StringValue(result.LocationConstraint)
    } else {
        //if getBucketLocation is null us-east-1 used
        //http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
        region = "us-east-1"
    }

    return region
}

func GetPublicBuckets(config aws.Config) (PublicBuckets, error) {
    var pubBucket PublicBuckets

    buckets, err := GetBuckets(config)
    if err != nil {
        if aerr, ok := err.(awserr.Error); ok {
            switch aerr.Code() {
            default:
                fmt.Println(aerr.Error())
            }
        } else {
            // Print the error, cast err to awserr.Error to get the Code and
            // Message from an error.
            fmt.Println(err.Error())
        }
        return pubBucket, err
    }

    for bucket := range buckets {
        region := GetBucketRegion(config, buckets[bucket]) // Required or we will get BucketRegionError
        svcTmp := s3.New(session.New(), &aws.Config{Region: aws.String(region)})

        input := &s3.GetBucketPolicyInput{}
        input.Bucket = aws.String(buckets[bucket])

        result, err := svcTmp.GetBucketPolicy(input)
        if err != nil {
            if aerr, ok := err.(awserr.Error); ok {
                switch aerr.Code() {
                case "NoSuchBucketPolicy":
                case "BucketRegionError":
                    fmt.Printf("%s: %s\n", buckets[bucket], aerr.Error())
                case "AccessDenied":
                    fmt.Printf("AccessDenied for bucket: %s\n", buckets[bucket])
                default:
                    fmt.Printf("%s: %s\n", buckets[bucket], aerr.Error())
                }
            } else {
                // Print the error, cast err to awserr.Error to get the Code and
                // Message from an error.
                fmt.Println(err.Error())
            }
        }

        openPolicy := OpenPolicy(result.Policy)
        if openPolicy {
            pubBucket.Policy = append(pubBucket.Policy, buckets[bucket])
        }

        aclInput := &s3.GetBucketAclInput{}
        aclInput.Bucket = aws.String(buckets[bucket])

        result2, err := svcTmp.GetBucketAcl(aclInput)
        if err != nil {
            continue
        }

        openACL := OpenACL(result2.Grants)
        if openACL {
            pubBucket.ACL = append(pubBucket.ACL, buckets[bucket])
        }
    }

    return pubBucket, nil
}
