// slurp s3 bucket enumerator
// Copyright (C) 2019 hehnope
//
// slurp is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// slurp is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Foobar. If not, see <http://www.gnu.org/licenses/>.
//

package intern

import (
    "encoding/json"
    "strings"
    "regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

    "github.com/jmoiron/jsonq"
    log "github.com/sirupsen/logrus"
)

// PublicBuckets stores a list of the public buckets either by ACL or by Policy
type PublicBuckets struct {
    ACL    []string
    Policy []string
}

// GetBuckets returns all of the buckets
func GetBuckets(config aws.Config) ([]string, error) {
	svc := s3.New(session.New(), &config)
	input := &s3.ListBucketsInput{}

	result, err := svc.ListBuckets(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				log.Error(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Error(err.Error())
		}
		return []string{}, err
	}

	var buckets []string

	for bucket := range result.Buckets {
		buckets = append(buckets, aws.StringValue(result.Buckets[bucket].Name))
	}

	return buckets, nil
}

// OpenPolicy checks a policy to see if its "open"
func OpenPolicy(policy *string) (bool) {
    data := map[string]interface{}{}
    dec := json.NewDecoder(strings.NewReader(aws.StringValue(policy)))
    dec.Decode(&data)
    jq := jsonq.NewQuery(data)

    effect, _ := jq.String("Statement", "0", "Effect")
    principal, _ := jq.String("Statement", "0", "Principal")
    action, _ := jq.String("Statement", "0", "Action")

    re := regexp.MustCompile("(s3:Get\\*|s3:Describe\\*|s3:List\\*|s3:GetObject)")

    if effect == "Allow" && principal == "*" && re.MatchString(action) {
        return true
    }

    return false
}

// OpenACL checks an ACL to see if its "open"
func OpenACL(grants []*s3.Grant) (bool) {
    for grant := range grants {
        //fmt.Println(aws.StringValue(grants[grant].Grantee.URI))
        //fmt.Println(aws.StringValue(grants[grant].Permission))

        re := regexp.MustCompile("(READ|WRITE|FULL_CONTROL)")

        if aws.StringValue(grants[grant].Grantee.URI) == "http://acs.amazonaws.com/groups/global/AllUsers" {
            if re.MatchString(aws.StringValue(grants[grant].Permission)) {
                return true
            }
        }
    }

    return false
}

// GetBucketRegion determines the region for a bucket
func GetBucketRegion(config aws.Config, bucket string) string {
    svc := s3.New(session.New(), &config)
    input := &s3.GetBucketLocationInput{}
    input.Bucket = aws.String(bucket)

    result, err := svc.GetBucketLocation(input)
    if err != nil {
        if aerr, ok := err.(awserr.Error); ok {
            switch aerr.Code() {
            case "AccessDenied":
                log.Errorf("AccessDenied for bucket: %s (using default: %s)\n", bucket, aws.StringValue(config.Region))
                return aws.StringValue(config.Region)
            default:
                log.Error(aerr.Error())
            }
        } else {
            // Print the error, cast err to awserr.Error to get the Code and
            // Message from an error.
            log.Error(err.Error())
        }
    }

    var region string

    //log.Infof("%s %s", result.String(), bucket)
    if strings.Contains(result.String(), "LocationConstraint") {
        region = aws.StringValue(result.LocationConstraint)
    } else {
        //if getBucketLocation is null us-east-1 used
        //http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
        region = "us-east-1"
    }

    return region
}

// GetPublicBuckets determines which buckets are public
func GetPublicBuckets(config aws.Config) (PublicBuckets, error) {
    var pubBucket PublicBuckets

    buckets, err := GetBuckets(config)
    if err != nil {
        if aerr, ok := err.(awserr.Error); ok {
            switch aerr.Code() {
            default:
                log.Error(aerr.Error())
            }
        } else {
            // Print the error, cast err to awserr.Error to get the Code and
            // Message from an error.
            log.Error(err.Error())
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
                    log.Errorf("%s: %s\n", buckets[bucket], aerr.Error())
                case "AccessDenied":
                    log.Errorf("AccessDenied for bucket: %s (skipping)\n", buckets[bucket])
                    continue
                case "AuthorizationHeaderMalformed":
                    log.Errorf("AuthorizationHeaderMalformed for bucket: %s (%s)\n", buckets[bucket], aerr.Error())
                default:
                    log.Errorf("%s: %s\n", buckets[bucket], aerr.Error())
                }
            } else {
                // Print the error, cast err to awserr.Error to get the Code and
                // Message from an error.
                log.Error(err.Error())
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
