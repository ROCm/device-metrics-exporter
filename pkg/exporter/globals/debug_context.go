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

package globals

import "context"

// contextKey is an unexported type for context keys to avoid collisions
type contextKey string

const debugModeContextKey contextKey = "debugMode"

// WithDebugMode returns a new context with the debug mode value
func WithDebugMode(ctx context.Context, mode DebugMode) context.Context {
	return context.WithValue(ctx, debugModeContextKey, mode)
}

// GetDebugMode extracts the debug mode from context, returns DebugModeNone if not found
func GetDebugMode(ctx context.Context) DebugMode {
	if mode, ok := ctx.Value(debugModeContextKey).(DebugMode); ok {
		return mode
	}
	return DebugModeNone
}
