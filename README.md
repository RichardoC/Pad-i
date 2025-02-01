# Pad-i

Yet another llm based assistant
## Overview

Pad-i is an AI assistant that can maintain conversations, store knowledge, and search through previous interactions. It uses a local SQLite database to persist conversations and knowledge, and integrates with LLM services like OpenAI or local LLMs.

## Features

- Chat interface with conversation history
- Knowledge base storage and search
- Multiple conversation support
- Web-based UI
- Support for different LLM backends

## Prerequisites

- Go 1.20 or later
- SQLite3
- Access to an LLM service (OpenAI API or local LLM)

## Installation

1. Clone the repository:


### Frontend Development

The frontend is a simple web application using vanilla JavaScript. The main files are:
- `web/templates/index.html` - Main HTML template
- `web/static/css/style.css` - Styling
- `web/static/js/main.js` - Frontend logic

## API Endpoints

- `POST /api/message` - Send a message in a conversation
- `GET /api/conversations` - List all conversations
- `GET /api/messages` - Get messages for a conversation
- `GET /api/knowledge/search` - Search the knowledge base

## License

This project is licensed under the GNU Lesser General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

For support, please open an issue in the GitHub repository.