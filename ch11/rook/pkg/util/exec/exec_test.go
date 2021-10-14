/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package exec

import (
	"os/exec"
	"testing"

	"github.com/pkg/errors"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kexec "k8s.io/utils/exec"
)

func Test_assertErrorType(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"unknown error type", args{err: errors.New("i don't know this error")}, ""},
		{"exec.exitError type", args{err: &exec.ExitError{Stderr: []byte("this is an error")}}, "this is an error"},
		{"exec.Error type", args{err: &exec.Error{Name: "my error", Err: errors.New("this is an error")}}, "exec: \"my error\": this is an error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assertErrorType(tt.args.err); got != tt.want {
				t.Errorf("assertErrorType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// import TestMockExecHelperProcess
func TestMockExecHelperProcess(t *testing.T) {
	exectest.TestMockExecHelperProcess(t)
}

func TestExtractExitCode(t *testing.T) {
	mockExecExitError := func(retcode int) *exec.ExitError {
		// we can't create an exec.ExitError directly, but we can get one by running a command that fails
		// use go's type assertion to be sure we are returning exactly *exec.ExitError
		err := exectest.MockExecCommandReturns(t, "stdout", "stderr", retcode)

		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to create an *exec.ExitError. instead %T", err)
		}
		return ee
	}

	expectError := true
	noError := false

	tests := []struct {
		name     string
		inputErr error
		want     int
		wantErr  bool
	}{
		{"*exec.ExitError",
			mockExecExitError(3),
			3, noError},
		/* {"exec.ExitError", // non-pointer case is impossible (won't compile) */
		{"*kexec.CodeExitError (pointer)",
			&kexec.CodeExitError{Err: errors.New("some error"), Code: 4},
			4, noError},
		{"kexec.CodeExitError (non-pointer)",
			kexec.CodeExitError{Err: errors.New("some error"), Code: 5},
			5, noError},
		{"*kerrors.StatusError",
			&kerrors.StatusError{ErrStatus: metav1.Status{Code: 6}},
			6, noError},
		/* {"kerrors.StatusError", // non-pointer case is impossible (won't compile) */
		{"unknown error type with error code extractable from error message",
			errors.New("command terminated with exit code 7"),
			7, noError},
		{"unknown error type with no extractable error code",
			errors.New("command with no extractable error code even with an int here: 8"),
			-1, expectError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractExitCode(tt.inputErr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractExitCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}
