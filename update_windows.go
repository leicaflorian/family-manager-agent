package main

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"os/exec"
)

// The name the program of before keeps until the next start: see replaceProgram.
const oldSuffix = ".old"

// updateTarget is the program in its own folder, the one the link starts (see install), also when
// this agent was opened from somewhere else.
func updateTarget() string {
	target, err := installPath()
	if err != nil {
		return ""
	}
	return target
}

// restartProgram starts the program again: this agent is still quitting, and the new one goes on
// without it.
func restartProgram(target string) error {
	return exec.Command(target).Start()
}

// replaceProgram takes the program out of the zip and puts it in the place of the old one. Windows
// does not let a running program be overwritten, but lets it be renamed: the old one is moved
// aside, and install deletes it at the next start. If the new one does not get there the old one
// goes back.
func replaceProgram(archive, target string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	var program *zip.File
	for _, file := range reader.File {
		if file.Name == programName {
			program = file
		}
	}
	if program == nil {
		return errors.New("the zip does not hold the program")
	}
	fresh := target + ".new"
	if err := extract(program, fresh); err != nil {
		os.Remove(fresh)
		return err
	}
	old := target + oldSuffix
	os.Remove(old)
	if err := os.Rename(target, old); err != nil && !os.IsNotExist(err) {
		os.Remove(fresh)
		return err
	}
	if err := os.Rename(fresh, target); err != nil {
		os.Rename(old, target)
		os.Remove(fresh)
		return err
	}
	return nil
}

func extract(file *zip.File, to string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(to)
	if err != nil {
		return err
	}
	defer target.Close()
	_, err = io.Copy(target, source)
	return err
}
