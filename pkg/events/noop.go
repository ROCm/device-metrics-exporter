/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package events

import "context"

// EventEmitter is the test-only contract for substituting the event backend.
type EventEmitter interface {
	EmitWarning(ctx context.Context, reason EventReason, msg string)
	EmitWarningSync(ctx context.Context, reason EventReason, msg string) error
}

// NoopEmitter discards all events. Useful for bare-metal and tests.
type NoopEmitter struct{}

func (NoopEmitter) EmitWarning(context.Context, EventReason, string)           {}
func (NoopEmitter) EmitWarningSync(context.Context, EventReason, string) error { return nil }

// RecordedEvent captures one emitted event for test assertions.
type RecordedEvent struct {
	Reason EventReason
	Msg    string
	Sync   bool
}

// RecordingEmitter captures emitted events for tests.
type RecordingEmitter struct {
	Events []RecordedEvent
}

func (r *RecordingEmitter) EmitWarning(_ context.Context, reason EventReason, msg string) {
	r.Events = append(r.Events, RecordedEvent{Reason: reason, Msg: msg})
}

func (r *RecordingEmitter) EmitWarningSync(_ context.Context, reason EventReason, msg string) error {
	r.Events = append(r.Events, RecordedEvent{Reason: reason, Msg: msg, Sync: true})
	return nil
}

var (
	_ EventEmitter = NoopEmitter{}
	_ EventEmitter = (*RecordingEmitter)(nil)
)
