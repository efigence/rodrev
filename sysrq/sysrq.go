package sysrq

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
)

var sysrqPath = "/proc/sysrq-trigger"
var sysrqConfigPath = "/proc/sys/kernel/sysrq"

// TODO validate whether called command is enabled in kernel

//   2 =   0x2 - enable control of console logging level
//   4 =   0x4 - enable control of keyboard (SAK, unraw)
//   8 =   0x8 - enable debugging dumps of processes etc.
//  16 =  0x10 - enable sync command
//  32 =  0x20 - enable remount read-only
//  64 =  0x40 - enable signalling of processes (term, kill, oom-kill)
// 128 =  0x80 - allow reboot/poweroff
// 256 = 0x100 - allow nicing of all RT tasks

var sysrqState = 0

const (
	CmdReadonly = 'u'
	CmdOff      = 'o'
	CmdSync     = 's'
	CmdTerm     = 'e'
	CmdReboot   = 'b'
)

func updateSysrqState() error {
	f, err := ioutil.ReadFile(sysrqConfigPath)
	if err != nil {
		return fmt.Errorf("error opening %s: %s", sysrqConfigPath, err)
	}
	state, err := strconv.Atoi(string(f))
	if err != nil {
		return fmt.Errorf("error parsing %s[%s]: %s", sysrqConfigPath, string(f), err)
	}
	sysrqState = state
	return nil

}

func Trigger(cmd rune) error {
	sysrq, err := os.OpenFile(sysrqPath, os.O_WRONLY|os.O_SYNC, 0200)
	if err != nil {
		return fmt.Errorf("error opening sysrq: %s", err)
	}
	n, err := sysrq.Write([]byte(string(cmd)))
	if err != nil {
		return fmt.Errorf("error sending sysrq: %s", err)
	}
	if n < 1 {
		return fmt.Errorf("sysrq write returned zero bytes")
	}
	return sysrq.Close()

}
