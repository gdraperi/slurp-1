// slurp s3 bucket enumerator
// Copyright (C) 2017 8c30ff1057d69a6a6f6dc2212d8ec25196c542acb8620eb4148318a4b10dd131
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//

package main

import (
	"os"

	log "github.com/sirupsen/logrus"

    "slurp/scanner/external"
    "slurp/scanner/cmd"
)

// Global config
var cfg cmd.Config

func main() {
    cfg = cmd.CmdInit("slurp", "Public buckets finder", "Public buckets finder")

	switch cfg.State {
	case "DOMAIN":
		external.Init(&cfg)

		log.Info("Building permutations....")
		external.PermutateDomainRunner(&cfg)

		log.Info("Processing permutations....")
		external.CheckDomainPermutations(&cfg)

	case "KEYWORD":
		external.Init(&cfg)

		log.Info("Building permutations....")
		external.PermutateKeywordRunner(&cfg)

		log.Info("Processing permutations....")
		external.CheckKeywordPermutations(&cfg)

	case "NADA":
		log.Info("Check help")
		os.Exit(0)
	}

	// Print stats info
	log.Printf("%+v", cfg.Stats)
}
