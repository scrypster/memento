// Memento Web UI - Alpine.js Application

Alpine.data('mementoApp', () => ({
  // Navigation
  currentView: 'search',
  currentMemoryId: null,

  // Search state
  searchQuery: '',
  searchResults: [],
  searchLoading: false,

  // Theme
  isDarkMode: true,

  // Connections
  activeConnection: localStorage.getItem('memento_connection') || 'default',
  connections: [],
  defaultConnection: 'default',
  connectionsLoading: false,
  addConnectionModalOpen: false,
  connectionEditMode: false,
  newConnection: {
    name: '',
    display_name: '',
    description: '',
    database: {
      type: 'sqlite',
      path: './data/',
      host: 'localhost',
      port: 5432,
      username: '',
      password: '',
      database: '',
      sslmode: 'disable'
    },
    llm: {
      provider: 'ollama',
      model: '',
      api_key: ''
    }
  },
  connectionCreating: false,
  connectionTesting: false,
  connectionTestResult: { success: false, message: '' },
  availableConnections: [],

  // Stats
  stats: {
    memories: 0,
    entities: 0,
    relationships: 0,
    queueSize: 0
  },

  // Relationships Modal
  showRelationshipsModal: false,
  relationships: [],
  relationshipsLoading: false,

  // WebSocket
  wsConnected: false,
  ws: null,
  reconnectAttempts: 0,

  // Notifications
  notifications: [],
  notificationId: 0,

  // Initialization
  init() {
    console.log('Memento Web UI initializing...');
    this.initTheme();
    this.connectWebSocket();
    this.loadConnections();
    this.loadStats();
    this.loadConnectionsList();
    this.handleRouting();
  },

  // WebSocket connection
  connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      console.log('WebSocket connected');
      this.wsConnected = true;
      this.reconnectAttempts = 0;
    };

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.handleWebSocketMessage(message);
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      this.wsConnected = false;
    };

    this.ws.onclose = () => {
      console.log('WebSocket disconnected');
      this.wsConnected = false;
      this.reconnect();
    };
  },

  reconnect() {
    if (this.reconnectAttempts >= 50) {
      this.addNotification('error', 'Lost connection to server. Refresh page to retry.');
      return;
    }

    const delay = Math.min(30000, 1000 * Math.pow(2, this.reconnectAttempts));
    this.reconnectAttempts++;

    setTimeout(() => {
      console.log(`Reconnecting... (attempt ${this.reconnectAttempts})`);
      this.connectWebSocket();
    }, delay);
  },

  handleWebSocketMessage(message) {
    console.log('WebSocket message:', message);

    // NOTE: Primary WebSocket handling is in index.html's handleWebSocketMessage.
    // This handler only covers the legacy Alpine search component (app.js).
    switch (message.type) {
      case 'stats_update':
        this.stats = message.data;
        break;
      case 'error':
        this.addNotification('error', message.data?.message || 'Unknown error');
        break;
    }
  },

  // API calls
  async apiCall(url, options = {}) {
    try {
      const response = await fetch(url, {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          ...options.headers
        }
      });

      const contentType = response.headers.get('Content-Type') || '';
      if (!contentType.includes('application/json')) {
        throw new Error(`Server returned unexpected content type: ${contentType}`);
      }

      if (!response.ok) {
        const error = await response.json().catch(() => ({}));
        throw new Error(error.message || error.error || `HTTP ${response.status}`);
      }

      return await response.json();
    } catch (error) {
      this.addNotification('error', error.message);
      console.error('API Error:', error);
      return null;
    }
  },

  async loadStats() {
    const data = await this.apiCall('/api/stats');
    if (data) {
      this.stats = data;
    }
  },

  async refreshStats() {
    await this.loadStats();
    this.addNotification('success', 'Stats refreshed');
  },

  async loadRelationships() {
    this.relationshipsLoading = true;
    this.relationships = [];
    const data = await this.apiCall('/api/relationships');
    if (data && data.relationships) {
      this.relationships = data.relationships;
    }
    this.relationshipsLoading = false;
  },

  getEntityIcon(type) {
    const icons = {
      'person': '👤',
      'tool': '🔧',
      'database': '🗄️',
      'organization': '🏢',
      'concept': '💡',
      'project': '📊',
      'skill': '⭐',
      'event': '📅',
      'location': '📍',
      'product': '🎁'
    };
    return icons[type] || '🔹';
  },

  getEntityTypeColor(type) {
    const colors = {
      'person': 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
      'tool': 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
      'database': 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300',
      'organization': 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
      'concept': 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300',
      'project': 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/30 dark:text-cyan-300',
      'skill': 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300',
      'event': 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
      'location': 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300',
      'product': 'bg-teal-100 text-teal-700 dark:bg-teal-900/30 dark:text-teal-300'
    };
    return colors[type] || 'bg-gray-100 text-gray-700 dark:bg-gray-700/30 dark:text-gray-300';
  },

  formatRelationshipDate(timestamp) {
    if (!timestamp) return '';
    try {
      const date = new Date(timestamp);
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return '';
    }
  },

  async performSearch() {
    if (!this.searchQuery) {
      this.searchResults = [];
      return;
    }

    this.searchLoading = true;
    const data = await this.apiCall(`/api/search?q=${encodeURIComponent(this.searchQuery)}`);
    if (data) {
      this.searchResults = data.results || [];
    }
    this.searchLoading = false;
  },

  // Navigation
  navigateTo(view, id = null) {
    this.currentView = view;
    this.currentMemoryId = id;
    window.scrollTo(0, 0);
    window.location.hash = id ? `#/${view}/${encodeURIComponent(id)}` : `#/${view}`;
  },

  handleRouting() {
    const updateView = () => {
      const hash = window.location.hash.slice(1) || '/';
      const match = hash.match(/^\/([^/]+)(?:\/(.+))?$/);

      if (match) {
        this.currentView = match[1] || 'search';
        this.currentMemoryId = match[2] ? decodeURIComponent(match[2]) : null;
      } else {
        this.currentView = 'search';
        this.currentMemoryId = null;
      }
    };

    window.addEventListener('hashchange', updateView);
    updateView();
  },

  // Notifications
  addNotification(type, message) {
    const id = `notif-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

    this.notifications.push({ id, type, message });

    // Auto-dismiss after 5s (except errors)
    if (type !== 'error') {
      setTimeout(() => {
        this.removeNotification(id);
      }, 5000);
    }
  },

  removeNotification(id) {
    this.notifications = this.notifications.filter(n => n.id !== id);
  },

  // Theme toggle
  // Dark mode: html.dark class (activates Tailwind dark: variants)
  // Light mode: html.light class (activates CSS variable overrides in theme.css)
  initTheme() {
    const theme = localStorage.getItem('theme');
    this.isDarkMode = theme !== 'light';
    if (theme === 'light') {
      document.documentElement.classList.add('light');
      document.documentElement.classList.remove('dark');
    } else {
      document.documentElement.classList.add('dark');
      document.documentElement.classList.remove('light');
    }
  },

  toggleTheme() {
    this.isDarkMode = !this.isDarkMode;
    if (this.isDarkMode) {
      // Switch to dark mode: add 'dark', remove 'light'
      document.documentElement.classList.add('dark');
      document.documentElement.classList.remove('light');
      localStorage.setItem('theme', 'dark');
    } else {
      // Switch to light mode: add 'light', remove 'dark'
      document.documentElement.classList.add('light');
      document.documentElement.classList.remove('dark');
      localStorage.setItem('theme', 'light');
    }
  },

  // Connections Management
  async loadConnections() {
    try {
      const response = await fetch('/api/connections');
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      this.availableConnections = data.connections || [
        { name: 'default', display_name: 'Default' }
      ];
      const connExists = this.availableConnections.some(c => c.name === this.activeConnection);
      if (!connExists && this.availableConnections.length > 0) {
        this.activeConnection = this.availableConnections[0].name;
        localStorage.setItem('memento_connection', this.activeConnection);
      }
      if (!this.activeConnection || this.activeConnection === '') {
        this.activeConnection = 'default';
        localStorage.setItem('memento_connection', 'default');
      }
    } catch (error) {
      console.warn('Failed to load connections, using default:', error);
      this.availableConnections = [
        { name: 'default', display_name: 'Default' }
      ];
      this.activeConnection = 'default';
    }
  },

  async switchConnection() {
    localStorage.setItem('memento_connection', this.activeConnection);
    this.addNotification('info', `Switched to connection: ${this.activeConnection}`);
    this.loadStats();
  },

  async loadConnectionsList() {
    this.connectionsLoading = true;
    try {
      const data = await this.apiCall('/api/connections');
      this.connections = data.connections || [];
      this.defaultConnection = data.default_connection || 'default';
    } catch (error) {
      console.error('Failed to load connections:', error);
      this.addNotification('error', 'Failed to load connections');
    } finally {
      this.connectionsLoading = false;
    }
  },

  openAddConnectionModal() {
    this.newConnection = {
      name: '',
      display_name: '',
      description: '',
      database: {
        type: 'sqlite',
        path: './data/',
        host: 'localhost',
        port: 5432,
        username: '',
        password: '',
        database: '',
        sslmode: 'disable'
      },
      llm: {
        provider: 'ollama',
        model: '',
        api_key: ''
      }
    };
    this.connectionEditMode = false;
    this.connectionTestResult = { success: false, message: '' };
    this.addConnectionModalOpen = true;
  },

  closeAddConnectionModal() {
    this.addConnectionModalOpen = false;
    this.connectionTestResult = { success: false, message: '' };
  },

  async testConnectionConfig() {
    this.connectionTesting = true;
    this.connectionTestResult = { success: false, message: '' };

    try {
      const testPayload = {
        name: this.newConnection.name || 'test',
        display_name: this.newConnection.display_name || 'Test',
        description: this.newConnection.description || '',
        enabled: true,
        database: this.newConnection.database,
        llm: this.newConnection.llm
      };

      const response = await this.apiCall('/api/connections/test', {
        method: 'POST',
        body: JSON.stringify(testPayload)
      });

      this.connectionTestResult = {
        success: response && response.success,
        message: response ? (response.success ? response.message : response.error) : 'Connection test failed'
      };
    } catch (error) {
      this.connectionTestResult = {
        success: false,
        message: error.message || 'Connection test failed'
      };
    } finally {
      this.connectionTesting = false;
    }
  },

  async createConnection() {
    this.connectionCreating = true;

    try {
      const payload = {
        name: this.newConnection.name,
        display_name: this.newConnection.display_name,
        description: this.newConnection.description,
        enabled: true,
        database: {
          type: this.newConnection.database.type,
          path: this.newConnection.database.path,
          host: this.newConnection.database.host,
          port: parseInt(this.newConnection.database.port) || 5432,
          username: this.newConnection.database.username,
          password: this.newConnection.database.password,
          database: this.newConnection.database.database,
          sslmode: this.newConnection.database.sslmode
        },
        llm: {
          provider: this.newConnection.llm.provider,
          model: this.newConnection.llm.model
        }
      };

      if (this.newConnection.llm.api_key) {
        payload.llm.api_key = this.newConnection.llm.api_key;
      }

      const method = this.connectionEditMode ? 'PUT' : 'POST';
      const url = this.connectionEditMode
        ? `/api/connections/${encodeURIComponent(payload.name)}`
        : '/api/connections';

      await this.apiCall(url, {
        method: method,
        body: JSON.stringify(payload)
      });

      this.addNotification('success',
        this.connectionEditMode ? 'Connection updated successfully' : 'Connection created successfully'
      );
      this.closeAddConnectionModal();
      await this.loadConnectionsList();
      await this.loadConnections();
    } catch (error) {
      this.addNotification('error', error.message || 'Failed to save connection');
    } finally {
      this.connectionCreating = false;
    }
  },

  async deleteConnection(name) {
    if (!confirm(`Are you sure you want to delete the connection "${name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      await this.apiCall(`/api/connections/${encodeURIComponent(name)}`, {
        method: 'DELETE'
      });

      this.addNotification('success', 'Connection deleted successfully');
      await this.loadConnectionsList();
    } catch (error) {
      this.addNotification('error', error.message || 'Failed to delete connection');
    }
  },

  async setDefaultConnection(name) {
    try {
      await this.apiCall('/api/connections/default', {
        method: 'POST',
        body: JSON.stringify({ name })
      });

      this.addNotification('success', `"${name}" set as default connection`);
      await this.loadConnectionsList();
    } catch (error) {
      this.addNotification('error', error.message || 'Failed to set default connection');
    }
  },

  editActiveConnection() {
    const activeConn = this.connections.find(c => c.name === this.activeConnection);
    if (!activeConn) return;

    this.newConnection = JSON.parse(JSON.stringify(activeConn));
    this.connectionEditMode = true;
    this.connectionTestResult = { success: false, message: '' };
    this.addConnectionModalOpen = true;
  },

  getActiveConnectionData() {
    return this.connections.find(c => c.name === this.activeConnection) || {};
  }
}));
