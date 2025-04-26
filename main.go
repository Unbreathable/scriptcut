package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

var client *genai.Client

type Timestamps struct {
}

func main() {
	if err := godotenv.Load(); err != nil {
		panic(err)
	}

	// Parse the video file passed in as an argument
	args := os.Args
	if len(args) < 3 {
		log.Fatal("Usage: scriptcut <video-file> <prompt>")
	}
	inputFile := args[1]
	prompt := strings.Join(args[2:], " ")
	outputFile := "audio.mp3"

	cmd := exec.Command("ffmpeg", "-i", inputFile, "-q:a", "0", "-map", "a", outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("ffmpeg conversion failed: %v", err)
	}
	fmt.Printf("Audio written to %s\n", outputFile)
	// Delete the audio file that was used with the Gemini API
	defer os.Remove("audio.mp3")

	// Create a new client for the gemini api
	ctx := context.Background()
	var err error
	client, err = genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Count tokens
	file, err := client.Files.UploadFromPath(
		ctx,
		"audio.mp3",
		&genai.UploadFileConfig{
			MIMEType: "audio/mp3",
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Files.Delete(ctx, file.Name, nil)

	// Poll until the video file is completely processed (state becomes ACTIVE).
	for file.State == genai.FileStateUnspecified || file.State != genai.FileStateActive {
		fmt.Println("Processing audio...")
		fmt.Println("File state:", file.State)
		time.Sleep(5 * time.Second)

		file, err = client.Files.Get(ctx, file.Name, nil)
		if err != nil {
			log.Fatal(err)
		}
	}

	// The system prompt
	systemPrompt := `
	Your job is to cut a video just using the audio of the video according to the user's prompt. 
	
	Provide the result in the following JSON format (this is an example):
	{"stamps":"00:01:00-00:02:00,00:02:00-00:03:00"}

	You can also use more accurate timestamps (with a max of two numbers behind the dot):
	{"stamps":"00:00:13.20-00:00:15.30"}

	Separate the timestamps with a comma, as in the examples above. DO NOT PUT THE JSON IN A CODE BLOCK.
	Don't cut out little breaks when they aren't huge: You shouldn't cut out less than a second of a break between different clips.
	`

	// Generate the actual response
	parts := []*genai.Part{
		genai.NewPartFromText(prompt),
		genai.NewPartFromURI(file.URI, file.MIMEType),
	}
	contents := []*genai.Content{
		genai.NewContentFromParts(parts, "user"),
	}

	// Actually prompt gemini to cut the video
	response, err := client.Models.GenerateContent(
		ctx,
		os.Getenv("GEMINI_MODEL"),
		contents,
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Text())

	// Parse the timestamps
	var outputJson struct {
		Stamps string `json:"stamps"`
	}
	if err := json.Unmarshal([]byte(response.Text()), &outputJson); err != nil {
		panic("Gemini gave invalid output... why do you have to do this to me?")
	}
	var cuts []struct {
		Start string
		End   string
	}
	for _, cut := range strings.Split(outputJson.Stamps, ",") {
		cutArgs := strings.Split(cut, "-")
		cuts = append(cuts, struct {
			Start string
			End   string
		}{
			Start: cutArgs[0],
			End:   cutArgs[1],
		})
	}

	// Generate all of the cuts
	listFile, err := os.Create("cut_files.txt")
	if err != nil {
		log.Fatalln("failed to create list file:", err)
	}
	defer func() {
		listFile.Close()
		os.Remove(listFile.Name())
	}()
	os.Mkdir(".cuts", os.ModeDir)
	for i, c := range cuts {

		// Extract the little part from the actual file
		outputPath := fmt.Sprintf(".cuts/cut_%d.mp4", i)
		cmd := exec.Command(
			"ffmpeg",
			"-ss", c.Start,
			"-to", c.End,
			"-i", inputFile,
			"-c", "copy",
			outputPath,
		)
		if err := cmd.Run(); err != nil {
			log.Fatalln("cut", i, "failed:", err)
		}

		// Add the cutted file to the list for ffmpeg
		if _, err := fmt.Fprintf(listFile, "file '%s'\n", outputPath); err != nil {
			log.Fatalln("failed writing to list file:", err)
		}
	}
	defer func() {
		os.RemoveAll(".cuts/")
	}()

	// Concat all the files using ffmpeg
	cmd = exec.Command(
		"ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile.Name(),
		"-c", "copy",
		"output.mp4",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("ffmpeg concat failed: %v – output: %s", err, output)
	}
}
