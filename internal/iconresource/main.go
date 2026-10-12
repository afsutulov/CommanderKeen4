// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fmt"
	"github.com/tc-hib/winres"
	"os"
)

func main() {
	f, err := os.Open("icon.ico")
	check(err)
	defer f.Close()
	icon, err := winres.LoadICO(f)
	check(err)
	rs := winres.ResourceSet{}
	check(rs.SetIcon(winres.ID(1), icon))
	for _, arch := range []winres.Arch{winres.ArchAMD64, winres.ArchARM64} {
		name := fmt.Sprintf("icon_windows_%s.syso", arch)
		out, err := os.Create(name)
		check(err)
		err = rs.WriteObject(out, arch)
		closeErr := out.Close()
		check(err)
		check(closeErr)
		fmt.Println(name)
	}
}
func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
