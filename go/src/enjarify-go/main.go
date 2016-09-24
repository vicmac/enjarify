// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"enjarify-go/dex"
	"enjarify-go/jvm"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func Read(name string) string {
	data, err := ioutil.ReadFile(name)
	check(err)
	return string(data)
}

func Write(name string, data string) {
	check(ioutil.WriteFile(name, []byte(data), os.ModePerm))
}

func translate(opts jvm.Options, dexs ...string) (map[string]string, []string, map[string]error) {
	classes := make(map[string]string)
	errors := make(map[string]error)
	ordkeys := []string{}

	for _, data := range dexs {
		dex := dex.Parse(data)
		for _, cls := range dex.Classes {
			unicode_name := Decode(cls.Name) + ".class"
			_, ok1 := classes[unicode_name]
			_, ok2 := errors[unicode_name]
			if ok1 || ok2 {
				fmt.Printf("Warning, duplicate class name %s\n", unicode_name)
				continue
			}

			if class_data, err := jvm.ToClassFile(cls, opts); err == nil {
				classes[unicode_name] = class_data
				ordkeys = append(ordkeys, unicode_name)
			} else {
				errors[unicode_name] = err
			}

			if (len(classes)+len(errors))%1000 == 0 {
				fmt.Printf("%d classes processed\n", len(classes)+len(errors))
			}
		}
	}
	return classes, ordkeys, errors
}

func writeToJar(fname string, classes map[string]string, ordkeys []string) {
	file, err := os.Create(fname)
	check(err)
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()
	for _, unicode_name := range ordkeys {
		data := classes[unicode_name]
		f, err := w.Create(unicode_name)
		check(err)
		_, err = f.Write([]byte(data))
		check(err)
	}
}

func readDexes(apkname string) (res []string) {
	r, err := zip.OpenReader(apkname)
	check(err)
	defer r.Close()

	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "classes") && strings.HasSuffix(f.Name, ".dex") {
			rc, err := f.Open()
			check(err)
			data, err := ioutil.ReadAll(rc)
			check(err)
			res = append(res, string(data))
			rc.Close()
		}
	}
	return
}

func main() {
	pout := flag.String("o", "", "Output .jar file. Default is [input-filename]-enjarify.jar.")
	pforce := flag.Bool("f", false, "Force overwrite. If output file already exists, this option is required to overwrite.")
	pfast := flag.Bool("fast", false, "Speed up translation at the expense of generated bytecode being less readable.")
	ptests := flag.Bool("runtests", false, "")
	phash := flag.Bool("hashtests", false, "")
	flag.Parse()
	inputfile := flag.Arg(0)

	if *ptests {
		runTests()
		return
	} else if *phash {
		hashTests()
		return
	}

	if inputfile == "" {
		fmt.Printf("Error, no input filename passed.\n")
		return
	}

	dexs := []string{}
	if strings.HasSuffix(strings.ToLower(inputfile), ".apk") {
		dexs = readDexes(inputfile)
	} else {
		dexs = []string{Read(inputfile)}
	}

	outname := *pout
	if outname == "" {
		s := inputfile[strings.LastIndex(inputfile, "/")+1:]
		s = s[:strings.LastIndex(s, ".")]
		outname = s + "-enjarify.jar"
	}

	mode := os.O_RDWR | os.O_CREATE
	if !*pforce {
		mode |= os.O_EXCL
	}
	outfile, err := os.OpenFile(outname, mode, os.FileMode(0666))
	if err != nil {
		fmt.Printf("Error, output file already exists and -f was not specified. To overwrite the output file, pass -f\n")
		return
	}

	opts := jvm.PRETTY
	if *pfast {
		opts = jvm.NONE
	}

	classes, ordkeys, errors := translate(opts, dexs...)
	writeToJar(outname, classes, ordkeys)
	outfile.Close()
	fmt.Printf("Output written to %s\n", outname)

	for name, error := range errors {
		fmt.Printf("%s %s\n", name, error.Error())
	}
	fmt.Printf("%d classes translated successfully, %d classes had errors\n", len(classes), len(errors))
}
