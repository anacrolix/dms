window.server_state = null;
var last_server_update = null;


function genericFormHandler(url) {
  return {
    async submitForm() {
      try {
        await fetch(url + ip_addr, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json'
          },
          body: {}
        })
      } catch (error) {
        alert("Something went wrong: " + JSON.stringify(error));
        return;
      }
      location.reload();
    }
  }
}

function fetchData() {
  return {
    items: {},
    loading: true,
    async fetchData() {
      try {
        const response = await fetch('/status');
        const data = await response.json();
        this.items = data;
        window.server_state = {};
        window.server_state.items = data;
        window.server_state.loading = false;
        window.last_server_update = new Date();

      } catch (error) {
        console.error('Error fetching data:', error);
      } finally {
        this.loading = false;
      }
    },
    init() {
      this.fetchData();
    }
  };
};

function updateData() {
  if (window.last_server_update != null && window.server_state != null) {
    now = new Date();
    if ((now - window.last_server_update) / 1000 < 3)
      return window.server_state;
  }

  return fetchData();
}

async function remove_blacklist(ip_addr) {

  if (!confirm('Are you sure you want to remove ' + ip_addr + " from the blacklist?"))
    return;

  try {
    const response = await fetch('/blacklist/' + ip_addr, {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json'
      }
    });
    const data = await response.json();
    alert(JSON.stringify(data));
    location.reload();
  } catch (error) {
    alert(JSON.stringify(data));
    console.error('Error:', error);
  }
}

async function remove_whitelist(ip_addr) {
  if (!confirm('Are you sure you want to add ' + ip_addr + " to the whitelist?"))
    return;

  try {
    const response = await fetch('/whitelist/' + ip_addr, {
      method: 'DELETE',
      headers: {
        'Content-Type': 'application/json'
      }
    });
    const data = await response.json();
    alert(JSON.stringify(data));
    location.reload();
  } catch (error) {
    alert(JSON.stringify(data));
    console.error('Error:', error);
  }
}

async function block(ip_addr) {
  if (!confirm('Are you sure you want to block ' + ip_addr + " ?"))
    return;

  try {
    const response = await fetch('/blacklist/' + ip_addr, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json'
      }
    });
    const data = await response.json();
    alert(JSON.stringify(data));
    location.reload();
  } catch (error) {
    alert(JSON.stringify(data));
    console.error('Error:', error);
  }
}

async function unblock(ip_addr) {
  if (!confirm('Are you sure you want to unblock ' + ip_addr + " ?"))
    return;

  try {
    const response = await fetch('/whitelist/' + ip_addr, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json'
      }
    });
    const data = await response.json();
    alert(JSON.stringify(data));
    location.reload();
  } catch (error) {
    alert(JSON.stringify(data));
    console.error('Error:', error);
  }
}