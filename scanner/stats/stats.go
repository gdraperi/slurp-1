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

package stats

import (
	"encoding/json"
)

// Stats stores stat data while server is running
type Stats struct {
	Requests200     uint64
	Requests403     uint64
	Requests404     uint64
	Requests503     uint64
	Requests200List []string
	Requests403List []string
	Requests404List []string
	Requests503List []string
}

// NewStats constructor for ServerStats
func NewStats() *Stats {
	ss := new(Stats)

	return ss
}

// IncRequests200 increments total http 200 requests
func (ss *Stats) IncRequests200() {
	ss.Requests200++
}

// IncRequests403 increments total http 403 requests
func (ss *Stats) IncRequests403() {
	ss.Requests403++
}

// IncRequests404 increments total http 404 requests
func (ss *Stats) IncRequests404() {
	ss.Requests404++
}

// IncRequests503 increments total http 503 requests
func (ss *Stats) IncRequests503() {
	ss.Requests503++
}

// Add200Link adds a link to the 200 requests list
func (ss *Stats) Add200Link(link string) {
	ss.Requests200List = append(ss.Requests200List, link)
}

// Add403Link adds a link to the 403 requests list
func (ss *Stats) Add403Link(link string) {
	ss.Requests403List = append(ss.Requests403List, link)
}

// Add404Link adds a link to the 404 requests list
func (ss *Stats) Add404Link(link string) {
	ss.Requests404List = append(ss.Requests404List, link)
}

// Add503Link adds a link to the 503 requests list
func (ss *Stats) Add503Link(link string) {
	ss.Requests503List = append(ss.Requests503List, link)
}

// JSONDump converts struct to json
func (ss *Stats) JSONDump() (string, error) {
	b, err := json.Marshal(ss)

	return string(b), err
}
