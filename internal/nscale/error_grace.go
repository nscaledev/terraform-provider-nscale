/*
Copyright 2026 Nscale

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nscale

import "time"

// ErrorGrace lets a waiter ride out an error the backend recovers from on its
// own: an unbroken run of failing polls is tolerated for up to Period.
type ErrorGrace struct {
	Period time.Duration

	since time.Time `exhaustruct:"optional"`
}

// Tolerate records one poll and reports whether a failing resource is still
// within Period. A non-failing poll ends the run.
func (g *ErrorGrace) Tolerate(failing bool) bool {
	if !failing {
		g.since = time.Time{}
		return false
	}
	if g.since.IsZero() {
		g.since = time.Now()
	}
	return time.Since(g.since) < g.Period
}
