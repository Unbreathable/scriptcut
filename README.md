# Cut videos with AI

When I first thought about this idea I was like: This is never going to work. But now after trying it, it's one of the best use cases I've found for AI so far. Wait for my upcoming video to see how powerful this can really be.

## Usage

### Requirements

- [Golang](https://go.dev/dl)
- FFMpeg (find out how to download it for your OS)
- A API key for the [Gemini API](https://ai.google.dev/gemini-api/docs)

### Installation

When you have everything required installed, you can install scriptcut using:

```sh
go install github.com/Unbreathable/scriptcut@latest
```

To run ScriptCut now, you can use the following command (you can also set your Gemini API key as an environment variable somewhere):

```sh
GEMINI_KEY=<key_here> scriptcut --help
```

If you can't run `scriptcut` yet, you might have to restart your Terminal.

If it still doesn't work, make sure you have [everything is set up correctly](https://go.dev/ref/mod#go-install).

You can look at the arguments the command needs by passing the `--help` option, you should be able to do everything from there.

## Contributing

I currently don't have any interest in developing this project any further. For this reason, I'll only ever look at pull requests / issues that fix bugs or, in the case of issues, ask questions about the project. If you want to create something on top of this, feel free to, that's why the project has the "Unlicense" License.
