/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package db

import (
	"bytes"
	"testing"
)

func TestJSONScanBytes(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		got, err := JSONScanBytes(nil)
		if err != nil || got != nil {
			t.Fatalf("nil: got=%v err=%v", got, err)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		in := []byte(`{"a":1}`)
		got, err := JSONScanBytes(in)
		if err != nil {
			t.Fatalf("bytes err: %v", err)
		}
		if !bytes.Equal(got, in) {
			t.Fatalf("bytes: got %q", got)
		}
	})

	t.Run("string", func(t *testing.T) {
		got, err := JSONScanBytes(`{"a":1}`)
		if err != nil {
			t.Fatalf("string err: %v", err)
		}
		if string(got) != `{"a":1}` {
			t.Fatalf("string: got %q", got)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		_, err := JSONScanBytes(123)
		if err == nil {
			t.Fatal("expected error for int")
		}
	})
}
