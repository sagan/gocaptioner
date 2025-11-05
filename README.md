Gemini LoRa Captioner

This is a handy command-line tool that automatically captions your images for you! It's built to make captions that are perfect for LoRa (or similar) training.

It uses the Google Gemini API to "look" at your images and write simple, comma-separated descriptions. It just drops a .txt file with the caption right next to each image.

Usage:

Set `GEMINI_API_KEY` env. Then run:

```
gocaptioner -dir .
```

For each image file in target dir, it generates a `<filename>.txt` file, example:

```
ohwx woman, a portrait of a stern-looking woman from the mid-19th century, wearing a dark dress and a white bonnet with lace trim, conveying a sense of solemnity and strict traditionalism
```

If `-identity` flag is set, it prepends it to the caption of each photo.

## Flags

```
gocaptioner.exe:
  -dir string
        Required: Path to the image directory
  -force
        Optional: Force re-generation of all captions, even if .txt files exist
  -identity string
        Optional: The trigger word (e.g., 'kongrongjin_3y') to prepend to each caption
```