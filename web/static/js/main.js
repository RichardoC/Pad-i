// Global state
let currentConversationId = null;

// Utility functions
function formatDate(dateString) {
    return new Date(dateString).toLocaleString();
}

async function fetchJSON(url, options = {}) {
    try {
        console.log(`Fetching ${url}...`);
        const response = await fetch(url, {
            ...options,
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            }
        });
        console.log(`Response status:`, response.status);
        
        if (!response.ok) {
            const errorText = await response.text();
            console.error('Response error:', errorText);
            throw new Error(`HTTP error! status: ${response.status}, body: ${errorText}`);
        }
        
        const data = await response.json();
        console.log(`Received data:`, data);
        return data;
    } catch (error) {
        console.error('Fetch error:', error);
        throw error;
    }
}

// Message handling
function createMessageElement(message) {
    const div = document.createElement('div');
    div.className = `message ${message.role}`;
    div.innerHTML = `
        <div class="message-header">
            <span class="role">${message.role}</span>
            <span class="time">${formatDate(message.created_at)}</span>
        </div>
        <div class="content">${message.content}</div>
    `;
    return div;
}

async function sendMessage() {
    const input = document.getElementById('userInput');
    const content = input.value.trim();
    if (!content || !currentConversationId) return;

    input.value = '';
    input.disabled = true;

    try {
        // Create and display user message immediately
        const userMessage = {
            role: 'user',
            content: content,
            created_at: new Date().toISOString()
        };
        const messagesDiv = document.getElementById('messages');
        messagesDiv.appendChild(createMessageElement(userMessage));
        messagesDiv.scrollTop = messagesDiv.scrollHeight;

        // Send message to server
        const response = await fetchJSON(`/api/message?conversation_id=${currentConversationId}`, {
            method: 'POST',
            body: JSON.stringify({ content })
        });

        // Add the assistant's response
        messagesDiv.appendChild(createMessageElement(response.message));

        // If a new conversation was created, switch to it
        if (response.new_conversation_id && response.new_conversation_id !== currentConversationId) {
            await loadConversations();
            switchConversation(response.new_conversation_id);
        }

        // Scroll to bottom
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
    } catch (error) {
        console.error('Error sending message:', error);
        alert('Failed to send message');
    } finally {
        input.disabled = false;
        input.focus();
    }
}

// Conversation handling
async function createNewConversation() {
    try {
        const title = prompt('Enter conversation title:');
        if (!title) return;

        const response = await fetchJSON('/api/conversations', {
            method: 'POST',
            body: JSON.stringify({ title })
        });

        await loadConversations();
        switchConversation(response.id);
    } catch (error) {
        console.error('Error creating conversation:', error);
        alert('Failed to create conversation');
    }
}

function createConversationElement(conversation) {
    const div = document.createElement('div');
    div.className = 'conversation';
    div.setAttribute('data-id', conversation.id);
    div.innerHTML = `
        <span class="title">${conversation.title}</span>
        <span class="time">${formatDate(conversation.created_at)}</span>
    `;
    div.onclick = () => switchConversation(conversation.id);
    return div;
}

async function loadConversations() {
    try {
        console.log('Fetching conversations...');
        const conversations = await fetchJSON('/api/conversations');
        console.log('Received conversations:', conversations);
        
        const listDiv = document.querySelector('.conversations-list');
        // Always show the "New Conversation" button
        listDiv.innerHTML = `
            <div class="new-conversation" onclick="createNewConversation()">
                + New Conversation
            </div>
        `;
        
        // Check if conversations is an array and has items
        if (Array.isArray(conversations)) {
            conversations.forEach(conv => {
                listDiv.appendChild(createConversationElement(conv));
            });
        } else {
            console.warn('Received invalid conversations data:', conversations);
        }
    } catch (error) {
        console.error('Error loading conversations:', error);
        console.error('Error details:', {
            message: error.message,
            stack: error.stack
        });
        alert('Failed to load conversations');
    }
}

async function switchConversation(conversationId) {
    currentConversationId = conversationId;
    
    // Update UI to show active conversation
    document.querySelectorAll('.conversation').forEach(el => {
        el.classList.toggle('active', el.getAttribute('data-id') === String(conversationId));
    });

    // Load messages for this conversation
    try {
        const messages = await fetchJSON(`/api/messages?conversation_id=${conversationId}`);
        const messagesDiv = document.getElementById('messages');
        messagesDiv.innerHTML = '';
        
        // Check if messages is an array
        if (Array.isArray(messages)) {
            if (messages.length === 0) {
                messagesDiv.innerHTML = '<div class="message system"><div class="content">No messages yet. Start the conversation!</div></div>';
            } else {
                messages.reverse().forEach(msg => {
                    messagesDiv.appendChild(createMessageElement(msg));
                });
            }
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        } else {
            console.warn('Received invalid messages data:', messages);
            messagesDiv.innerHTML = '<div class="message system"><div class="content">Failed to load messages</div></div>';
        }
    } catch (error) {
        console.error('Error loading messages:', error);
        document.getElementById('messages').innerHTML = 
            '<div class="message system"><div class="content">Failed to load messages</div></div>';
    }
}

// Knowledge base search
let searchTimeout = null;

function createSearchResultElement(result) {
    const div = document.createElement('div');
    div.className = 'search-result';
    div.innerHTML = `
        <div class="content">${result.content}</div>
        <div class="metadata">
            <span class="relevance">Relevance: ${(result.relevance * 100).toFixed(1)}%</span>
            <span class="time">${formatDate(result.created_at)}</span>
        </div>
    `;
    div.onclick = () => {
        const textarea = document.getElementById('userInput');
        textarea.value += `\n\nReferencing: ${result.content}`;
        textarea.focus();
    };
    return div;
}

async function searchKnowledge(query) {
    if (!query.trim()) {
        document.getElementById('searchResults').innerHTML = '';
        return;
    }

    try {
        const results = await fetchJSON(`/api/knowledge/search?q=${encodeURIComponent(query)}`);
        const searchResultsDiv = document.getElementById('searchResults');
        searchResultsDiv.innerHTML = '';
        
        if (results.length === 0) {
            searchResultsDiv.innerHTML = '<div class="no-results">No results found</div>';
            return;
        }

        results.forEach(result => {
            searchResultsDiv.appendChild(createSearchResultElement(result));
        });
    } catch (error) {
        console.error('Error searching knowledge:', error);
        document.getElementById('searchResults').innerHTML = 
            '<div class="error">Failed to search knowledge base</div>';
    }
}

// Event listeners
document.addEventListener('DOMContentLoaded', () => {
    loadConversations();
    
    // Handle Enter key in textarea
    document.getElementById('userInput').addEventListener('keypress', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    });

    // Add knowledge search handler
    const searchInput = document.getElementById('knowledgeSearch');
    searchInput.addEventListener('input', (e) => {
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(() => {
            searchKnowledge(e.target.value);
        }, 300); // Debounce search requests
    });

    // Add Ctrl+K shortcut for search focus
    document.addEventListener('keydown', (e) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
            e.preventDefault();
            searchInput.focus();
        }
    });
}); 