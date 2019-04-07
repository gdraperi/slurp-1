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

package main

import (
    "github.com/aws/aws-sdk-go/aws"
    log "github.com/sirupsen/logrus"

    "slurp/scanner/external"
    "slurp/scanner/cmd"
    "slurp/scanner/intern"
)

// Global config
var cfg cmd.Config

func main() {
    cfg = cmd.Init("slurp", "Public buckets finder", "Public buckets finder")

    switch cfg.State {
    case "DOMAIN":
        external.Init(&cfg)

        log.Info("Building permutations....")
        go external.PermutateDomainRunner(&cfg)

        log.Info("Processing permutations....")
        external.CheckDomainPermutations(&cfg)

        // Print stats info
        log.Printf("%+v", cfg.Stats)
    case "KEYWORD":
        external.Init(&cfg)

        log.Info("Building permutations....")
        go external.PermutateKeywordRunner(&cfg)

        log.Info("Processing permutations....")
        external.CheckKeywordPermutations(&cfg)

        // Print stats info
        log.Printf("%+v", cfg.Stats)
    case "INTERNAL":
        var config aws.Config
        config.Region = &cfg.Region

        log.Info("Determining public buckets....")
        buckets, err3 := intern.GetPublicBuckets(config)
        if err3 != nil {
            log.Error(err3)
        }

        for bucket := range buckets.ACL {
            log.Infof("S3 public bucket (ACL): %s", buckets.ACL[bucket])
        }

        for bucket := range buckets.Policy {
            log.Infof("S3 public bucket (Policy): %s", buckets.Policy[bucket])
        }

    default:
        log.Fatal("Check help")
    }
}
