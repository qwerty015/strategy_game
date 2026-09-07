// iconbuild creates Windows resources from the editable application PNG.
package main

import (
	"github.com/tc-hib/winres"
	"image/png"
	"log"
	"os"
)

func main() {
	if len(os.Args) != 5 {
		log.Fatal("usage: iconbuild input.png output.syso output.ico arch")
	}
	arch, ok := map[string]winres.Arch{"amd64": winres.ArchAMD64, "386": winres.ArchI386, "arm64": winres.ArchARM64}[os.Args[4]]
	if !ok {
		log.Fatal("unsupported Windows architecture")
	}
	f, err := os.Open(os.Args[1])
	check(err)
	img, err := png.Decode(f)
	f.Close()
	check(err)
	icon, err := winres.NewIconFromResizedImage(img, []int{16, 24, 32, 48, 64, 128, 256})
	check(err)
	var rs winres.ResourceSet
	check(rs.SetIcon(winres.ID(1), icon))
	coff, err := os.Create(os.Args[2])
	check(err)
	err = rs.WriteObject(coff, arch)
	closeErr := coff.Close()
	check(err)
	check(closeErr)
	ico, err := os.Create(os.Args[3])
	check(err)
	err = icon.SaveICO(ico)
	closeErr = ico.Close()
	check(err)
	check(closeErr)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
