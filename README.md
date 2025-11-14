goaider

a CLI aider tool to help some common works in AIGC.

## Usage

### Captioning images

Set `GEMINI_API_KEY` env. Then run:

```
goaider caption --dir .
```

For each image file in target dir, it generates a `<filename>.txt` file, example:

```
pink puffer jacket, faux fur collar, black pants, white bunny slippers, black hair, two pigtails, pink bunny hair ties, standing, holding white fluffy toy
```

If `--identity` flag is set, it prepends it to the caption of each photo.

### Cropping images

This command crops and resizes all images in a specified directory.
It crops images using [smartcrop](https://github.com/muesli/smartcrop).
By default it generate 1024x1024 output images, which is the best size for SDXL / FLUX training.

```
goaider crop --dir .
```

## Flags

### `caption`

```
goaider caption:
      --dir string        Required: Path to the image directory
      --force             Optional: Force re-generation of all captions, even if .txt files exist
      --identity string   Optional: The trigger word (e.g., 'kongrongjin_3y') to prepend to each caption
```

### `crop`

```
goaider crop:
      --dir string        Required: Path to the image directory
      --output string     Optional: output dir name. default to "<input-dir>-crop"
      --width int         Optional: target photo width. default: 1024.
      --height int        Optional: target photo height. default: 1024.
      --force             Optional bool flag. Process and generate the target output file even the same name file already exists.
```