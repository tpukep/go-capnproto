package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tpukep/bambam/bam"
)

var (
	outdir       = flag.String("o", ".", "specify output directory")
	source       = flag.String("source", "", "specify output directory")
	verbose      = flag.Bool("verbose", false, "verbose mode")
	structTypeRe = regexp.MustCompile("struct ([[:alpha:]]+)")
	capnpRe      = regexp.MustCompile("(?m)[\r\n]+^.*" + regexp.QuoteMeta(CAPNP_CODEC_ANT) + ".*$")
	msgpRe       = regexp.MustCompile("(?m)[\r\n]+^.*" + regexp.QuoteMeta(MSGP_CODEC_ANT) + ".*$")
)

const (
	CAPNP_CODEC_ANT = "$Codec.capnp;"
	MSGP_CODEC_ANT  = "$Codec.msgp;"
	SELF_PKG_NAME   = "github.com/tpukep/caps"
)

func use() {
	fmt.Fprintf(os.Stderr, "\nuse: caps -o <outdir> -source=<model.capnp>\n")
	fmt.Fprintf(os.Stderr, "     # Tool reads .capnp files and writes: go structs with json tags, capn'proto code, translation code, msgp code.\n")
	fmt.Fprintf(os.Stderr, "     # options:\n")
	fmt.Fprintf(os.Stderr, "     #   -o=\"outdir\" specifies the directory to write to (created if need be).\n")
	fmt.Fprintf(os.Stderr, "     #   -verbose=true enables verbose mode \n")
	fmt.Fprintf(os.Stderr, "     # required:\n")
	fmt.Fprintf(os.Stderr, "     #   -source=model.capnp specifies input schema file\n")
	fmt.Fprintf(os.Stderr, "     #\n")
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}

func main() {
	flag.Parse()

	flag.Usage = use
	if *source == "" {
		use()
	}

	source := *source
	sourceName := strings.TrimSuffix(source, ".capnp")

	pkg, err := build.Import(SELF_PKG_NAME, "./", build.FindOnly)
	if err != nil {
		fmt.Println("Failed to detect self package location:", err)
		os.Exit(1)
	}

	capsSchemaPath := fmt.Sprintf("-I%s", pkg.Dir)
	goSchemaPath := fmt.Sprintf("-I%s/vendor/github.com/glycerine/go-capnproto", pkg.Dir)

	capnpArgs := []string{capsSchemaPath, goSchemaPath, "compile", "-opgo"}

	sourceData, err := ioutil.ReadFile(source)
	if err != nil {
		fmt.Println("Failed to read schema file:", err)
		os.Exit(1)
	}

	sourceContent := string(sourceData)
	// Remove comments
	re := regexp.MustCompile("(?s)#.*?\n")
	cleanContent := re.ReplaceAllString(sourceContent, "\n")

	// Find codec annotations
	capnpGen := capnpRe.MatchString(cleanContent)
	msgpGen := msgpRe.MatchString(cleanContent)

	if capnpGen {
		capnpArgs = append(capnpArgs, "-ogo")
	}

	if *outdir != "." {
		if !bam.DirExists(*outdir) {
			err := os.MkdirAll(*outdir, 0755)
			if err != nil {
				fmt.Println("Failed to create output dir:", err)
				os.Exit(1)
			}
		}

		for i := 0; i < len(capnpArgs); i++ {
			capnpArgs[i] += *outdir
		}
	}
	capnpArgs = append(capnpArgs, source)

	// Generate plain Go code
	cmd := exec.Command("capnp", capnpArgs...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if *verbose {
		fmt.Printf("Executing: %q\n", strings.Join(cmd.Args, " "))
	}

	err = cmd.Run()
	if err != nil {
		fmt.Println("Failed to run Plain go code generator:", err)
		os.Exit(1)
	}

	if capnpGen {
		// Add suffix "Capn" to Capn'proto types
		outFilename := filepath.Join(*outdir, sourceName+".capnp.go")

		data, err := ioutil.ReadFile(outFilename)
		if err != nil {
			fmt.Println("Failed to read Capn'proto out file:", outFilename, "Error:", err)
			os.Exit(1)
		}

		content := string(data)
		matches := structTypeRe.FindAllStringSubmatch(sourceContent, -1)
		for _, match := range matches {
			typeName := match[1]
			if typeName != "" {
				content = strings.Replace(content, typeName, typeName+"Capn", -1)
			}
		}

		err = ioutil.WriteFile(outFilename, []byte(content), 0644)
		if err != nil {
			fmt.Println("Failed to run write replaced Capn'proto code file:", err)
			os.Exit(1)
		}
	}

	// Generate Msgp code
	if msgpGen {
		inFilename := filepath.Join(*outdir, sourceName+".go")
		outFilename := filepath.Join(*outdir, sourceName+".msgp.go")
		cmd = exec.Command("msgp", "-o="+outFilename, "-tests=false", "-file="+inFilename)
		cmd.Stderr = os.Stderr

		if *verbose {
			cmd.Stdout = os.Stdout
			fmt.Printf("Executing: %q\n", strings.Join(cmd.Args, " "))
		}

		err = cmd.Run()
		if err != nil {
			fmt.Println("Failed to run Msgp go code generator:", err)
			os.Exit(1)
		}
	}
}
