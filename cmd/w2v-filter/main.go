// This is a one-off tool to filter a list of words (e.g. Peter Norvig's list
// of 1/3 million most frequent english words [1]) based on which ones appear in
// the corpuses of the W2V models we're using.
//
// [1] https://norvig.com/ngrams/count_1w.txt
//
// Example invocation:
//
// GLOVE_MODEL_PATH=data/glove.bin \
// CONCEPT_NET_MODEL_PATH=data/conceptnet.bin \
// COMMON_WORDLIST=data/common_words_unfiltered.txt \
// OUTPUT_PATH=data/common_words_filtered.txt \
// go run ./cmd/w2v-filter/
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"code.sajari.com/word2vec"
	ff "github.com/peterbourgon/ff/v4"
)

func main() {
	if err := run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("no args given")
	}

	fSet := flag.NewFlagSet(args[0], flag.ContinueOnError)
	var (
		gloveModelPath      = fSet.String("glove_model_path", "", "Path to binary Word2Vec glove model data")
		conceptNetModelPath = fSet.String("concept_net_model_path", "", "Path to binary Word2Vec ConceptNet model data")
		commonWordlist      = fSet.String("common_wordlist", "", "Path to word list of most common words")
		outputPath          = fSet.String("output_path", "out.txt", "Output location where words should be written")
	)
	if err := ff.Parse(fSet, args[1:], ff.WithEnvVars()); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	gloveModel, err := loadModel(*gloveModelPath)
	if err != nil {
		return fmt.Errorf("failed to load glove model: %w", err)
	}

	conceptNetModel, err := loadModel(*conceptNetModelPath)
	if err != nil {
		return fmt.Errorf("failed to load conceptNet model: %w", err)
	}

	commonFile, err := os.Open(*commonWordlist)
	if err != nil {
		return fmt.Errorf("failed to open common word list: %w", err)
	}
	defer commonFile.Close()

	outFile, err := os.Create(*outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	sc := bufio.NewScanner(commonFile)
	n := 0
	for sc.Scan() {
		if n > 50000 {
			break
		}
		n++
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			log.Printf("line %q didn't have two fields: %+v", sc.Text(), fields)
			continue
		}
		line := fields[0]
		e := word2vec.Expr{}
		e.Add(1, line)
		if _, err := gloveModel.Eval(e); err != nil {
			log.Printf("glove didn't contain %q", line)
			continue
		}
		if _, err := conceptNetModel.Eval(e); err != nil {
			log.Printf("concept net didn't contain %q", line)
			continue
		}
		fmt.Fprintln(outFile, line)
	}

	if err := outFile.Close(); err != nil {
		return fmt.Errorf("failed to close output file: %w", err)
	}

	return nil
}

func loadModel(file string) (*word2vec.Model, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open model file %q: %w", file, err)
	}
	defer f.Close()

	model, err := word2vec.FromReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse model file %q: %w", file, err)
	}
	return model, nil
}
