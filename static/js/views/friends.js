import { api } from '../api.js';
import { templateLoader } from '../template.js';

let searchDebounceTimeout = null;

export async function render(container, router) {
    if (!api.isAuthenticated()) {
        router.navigate('/login');
        return;
    }

    const template = await templateLoader.load('friends');
    container.innerHTML = template;

    // Back to lobby button
    container.querySelector('#backToLobbyBtn').addEventListener('click', () => {
        router.navigate('/lobby');
    });

    // User search with debounce
    const searchInput = container.querySelector('#userSearchInput');
    searchInput.addEventListener('input', (e) => {
        clearTimeout(searchDebounceTimeout);
        searchDebounceTimeout = setTimeout(() => searchUsers(container, e.target.value), 300);
    });

    // Close search results on outside click
    document.addEventListener('click', (e) => {
        const searchResults = container.querySelector('#searchResults');
        const searchContainer = container.querySelector('.search-container');
        if (searchResults && searchContainer && !searchContainer.contains(e.target)) {
            searchResults.style.display = 'none';
        }
    });

    // Load friends data
    loadFriendsData(container);
}

export function cleanup() {
    if (searchDebounceTimeout) {
        clearTimeout(searchDebounceTimeout);
        searchDebounceTimeout = null;
    }
}

async function loadFriendsData(container) {
    try {
        const [friends, requests] = await Promise.all([
            api.getFriends(),
            api.getPendingRequests()
        ]);

        displayFriendsList(container, friends);
        displayFriendRequests(container, requests);
    } catch (error) {
        console.error('Failed to load friends data:', error);
        showMessage(container, 'Failed to load friends data', 'error');
    }
}

function displayFriendsList(container, friends) {
    const friendsList = container.querySelector('#friendsList');
    if (!friendsList) return;

    if (!friends || friends.length === 0) {
        friendsList.innerHTML = '<div class="empty-state">No friends yet</div>';
        return;
    }

    friendsList.innerHTML = friends.map(friend => `
        <div class="friends-page-item" data-user-id="${friend.userId}">
            <span class="username">${friend.username}</span>
        </div>
    `).join('');
}

function displayFriendRequests(container, requests) {
    const requestsSection = container.querySelector('#requestsSection');
    const requestsList = container.querySelector('#requestsList');
    const requestsCount = container.querySelector('#requestsCount');

    if (!requestsSection || !requestsList) return;

    if (!requests || requests.length === 0) {
        requestsSection.style.display = 'none';
        return;
    }

    requestsSection.style.display = 'block';
    requestsCount.textContent = requests.length;

    requestsList.innerHTML = requests.map(req => `
        <div class="friends-page-item" data-from-id="${req.fromUserId}">
            <span class="username">${req.fromUsername}</span>
            <div class="actions">
                <button class="accept-btn primary-btn" data-user-id="${req.fromUserId}">Accept</button>
                <button class="decline-btn secondary-btn" data-user-id="${req.fromUserId}">Decline</button>
            </div>
        </div>
    `).join('');

    // Add event listeners
    requestsList.querySelectorAll('.accept-btn').forEach(btn => {
        btn.addEventListener('click', () => acceptFriendRequest(container, parseInt(btn.dataset.userId)));
    });

    requestsList.querySelectorAll('.decline-btn').forEach(btn => {
        btn.addEventListener('click', () => declineFriendRequest(container, parseInt(btn.dataset.userId)));
    });
}

async function searchUsers(container, query) {
    const searchResults = container.querySelector('#searchResults');
    if (!searchResults) return;

    if (query.length < 2) {
        searchResults.style.display = 'none';
        return;
    }

    try {
        const users = await api.searchUsers(query);
        displaySearchResults(container, users);
    } catch (error) {
        console.error('User search failed:', error);
        searchResults.innerHTML = '<div class="search-error">Search failed</div>';
        searchResults.style.display = 'block';
    }
}

function displaySearchResults(container, users) {
    const searchResults = container.querySelector('#searchResults');
    if (!searchResults) return;

    if (!users || users.length === 0) {
        searchResults.innerHTML = '<div class="no-results">No users found</div>';
        searchResults.style.display = 'block';
        return;
    }

    searchResults.innerHTML = users.map(user => `
        <div class="search-result-item" data-user-id="${user.userId}">
            <span class="result-username">${user.username}</span>
            <button class="add-friend-btn" data-user-id="${user.userId}">Add</button>
        </div>
    `).join('');

    searchResults.style.display = 'block';

    // Add event listeners
    searchResults.querySelectorAll('.add-friend-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            sendFriendRequest(container, parseInt(btn.dataset.userId));
        });
    });
}

async function sendFriendRequest(container, userId) {
    try {
        await api.sendFriendRequest(userId);
        // Clear search
        const searchInput = container.querySelector('#userSearchInput');
        const searchResults = container.querySelector('#searchResults');
        if (searchInput) searchInput.value = '';
        if (searchResults) searchResults.style.display = 'none';
        showMessage(container, 'Friend request sent!', 'success');
    } catch (error) {
        console.error('Failed to send friend request:', error);
        showMessage(container, error.message || 'Failed to send request', 'error');
    }
}

async function acceptFriendRequest(container, friendId) {
    try {
        await api.acceptFriendRequest(friendId);
        loadFriendsData(container);
        showMessage(container, 'Friend request accepted!', 'success');
    } catch (error) {
        console.error('Failed to accept friend request:', error);
        showMessage(container, error.message || 'Failed to accept', 'error');
    }
}

async function declineFriendRequest(container, friendId) {
    try {
        await api.declineFriendRequest(friendId);
        loadFriendsData(container);
        showMessage(container, 'Friend request declined', 'success');
    } catch (error) {
        console.error('Failed to decline friend request:', error);
        showMessage(container, error.message || 'Failed to decline', 'error');
    }
}

function showMessage(container, message, type) {
    const msgDiv = container.querySelector('#friendsMessage');
    if (!msgDiv) return;

    msgDiv.className = type;
    msgDiv.textContent = message;

    // Auto-hide after 3 seconds
    setTimeout(() => {
        msgDiv.textContent = '';
        msgDiv.className = '';
    }, 3000);
}
