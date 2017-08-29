// +build integration

/*
Copyright (C) 2017 Red Hat, Inc.

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

package util

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"io"
	"strings"
)

var (
	terminal *exec.Cmd
	outbuf   bytes.Buffer
	errbuf   bytes.Buffer
	inbuf    bytes.Buffer
	
	outPipe io.ReadCloser
	errPipe io.ReadCloser
	inPipe io.WriteCloser

	outScanner *bufio.Scanner
	errScanner *bufio.Scanner

	stdoutChannel chan string
	stderrChannel chan string
	exitCodeChannel chan string
)

func StartTerminal() error {
	stdoutChannel = make(chan string)
	stderrChannel = make(chan string)
	exitCodeChannel = make(chan string)

	terminal = exec.Command("bash")

	outPipe, _ = terminal.StdoutPipe()
	errPipe, _ = terminal.StderrPipe()
	inPipe, _ = terminal.StdinPipe()

	outScanner = bufio.NewScanner(outPipe)
	errScanner = bufio.NewScanner(errPipe)

	go scanPipe(outScanner, &outbuf, "stdout", stdoutChannel)
	go scanPipe(errScanner, &errbuf, "stderr", stderrChannel)

	terminal.Start()
	
	return nil
}

func CloseTerminal() error {
	io.WriteString(inPipe, "exit\n")
	err := terminal.Wait()
	if err != nil {
		fmt.Println("error closing terminal:", err)
	}

	return err
}

func ExecuteInShell(command string) error {
	var errorValue error

	//this checks for exit code, will be different for every shell
	checkCommand := "echo exitCode=$?\n"
	
	inPipe.Write([]byte(command + "\n")) //actuall command
	inPipe.Write([]byte(checkCommand)) //check for its exit code
	
	exitCode := <- exitCodeChannel //will block until exit code is returned
	if exitCode != "0" {
		errorValue = fmt.Errorf("Command exited with non-zero exit code: %s\n", exitCode)
	}

	return errorValue
}


func scanPipe(scanner *bufio.Scanner, buffer *bytes.Buffer, stdType string, channel chan string) {
	for scanner.Scan() {
		
		str := scanner.Text()
		LogMessage(stdType, str)
		buffer.WriteString(str + "\n")
		fmt.Println(stdType + ">>>" + str)
		
		if strings.Contains(str, "exitCode=") {
			exitCode := strings.Split(str, "=")[1]
			exitCodeChannel <- exitCode
		}
	}

	return
}
