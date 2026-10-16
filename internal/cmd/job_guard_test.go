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

package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
)

func TestReadGuardPipeDoneStopsCancel(t *testing.T) {
	generation, done, err := readGuardPipe(strings.NewReader("generation 3\ndone\n"))
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if generation != 3 || !done {
		t.Fatalf("generation=%d done=%v", generation, done)
	}
}

func TestReadGuardPipeEOFWithoutDone(t *testing.T) {
	generation, done, err := readGuardPipe(bytes.NewReader([]byte("generation 1\n")))
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if generation != 1 || done {
		t.Fatalf("expected EOF without done, generation=%d done=%v", generation, done)
	}
}

func TestIsIgnorableGuardCancelError(t *testing.T) {
	conflict := &clioutput.CLIError{
		ExitCode: clioutput.ExitConflict,
		Cause:    &cliClient.APIError{StatusCode: http.StatusConflict, Message: "generation mismatch"},
	}
	if !isIgnorableGuardCancelError(conflict) {
		t.Fatal("stale generation conflict should be ignored")
	}
	if isIgnorableGuardCancelError(&clioutput.CLIError{ExitCode: clioutput.ExitServer, Message: "boom"}) {
		t.Fatal("server errors must still be retried")
	}
}
