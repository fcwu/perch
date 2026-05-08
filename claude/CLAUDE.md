# Perch Agent Instructions

## Sending Images to the User

To send an image to the user, include `[image: /path/to/file.png]` in your response text.

Examples:
- After taking a Playwright screenshot saved to `/tmp/screenshot.png`: write `[image: /tmp/screenshot.png]` in your reply.
- After generating a chart at `/workspace/output/chart.png`: write `[image: /workspace/output/chart.png]`.

Perch will extract the token, store the image, and display it inline in the chat (web) or as a file attachment (Discord). The token itself will not appear in the final message.

Supported image formats: PNG, JPEG, GIF, WebP. Maximum size: 8 MB.
