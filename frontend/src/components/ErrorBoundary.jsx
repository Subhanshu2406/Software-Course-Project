import { Component } from 'react'

export class ErrorBoundary extends Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, error: null, errorInfo: null }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true }
  }

  componentDidCatch(error, errorInfo) {
    console.error('Error caught by boundary:', error, errorInfo)
    this.setState({
      error,
      errorInfo,
    })
  }

  render() {
    if (this.state.hasError) {
      return (
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          backgroundColor: '#0a0e27',
          color: '#fff',
          fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
          flexDirection: 'column',
          padding: '20px',
        }}>
          <div style={{
            maxWidth: '600px',
            backgroundColor: '#1a1f3a',
            padding: '30px',
            borderRadius: '8px',
            border: '1px solid #e74c3c',
          }}>
            <h1 style={{ color: '#e74c3c', marginTop: 0 }}>Application Error</h1>
            <p>The frontend encountered an error. This usually means backend services aren't responding.</p>

            <div style={{
              backgroundColor: '#0a0e27',
              padding: '15px',
              borderRadius: '4px',
              fontSize: '12px',
              fontFamily: 'monospace',
              overflow: 'auto',
              maxHeight: '300px',
              marginBottom: '20px',
            }}>
              <strong style={{ color: '#e74c3c' }}>Error:</strong><br />
              {this.state.error?.toString()}
            </div>

            <details style={{ marginBottom: '20px', color: '#aaa' }}>
              <summary style={{ cursor: 'pointer', marginBottom: '10px' }}>Troubleshooting Steps</summary>
              <ol style={{ margin: '10px 0', paddingLeft: '20px', fontSize: '14px' }}>
                <li>Check Docker containers are running:
                  <code style={{
                    display: 'block',
                    background: '#0a0e27',
                    padding: '8px',
                    margin: '5px 0',
                    borderRadius: '4px',
                  }}>docker compose ps</code>
                </li>
                <li>Ensure all containers show "Healthy":
                  <code style={{
                    display: 'block',
                    background: '#0a0e27',
                    padding: '8px',
                    margin: '5px 0',
                    borderRadius: '4px',
                  }}>docker compose ps | grep healthy</code>
                </li>
                <li>Check backend services are responding:
                  <code style={{
                    display: 'block',
                    background: '#0a0e27',
                    padding: '8px',
                    margin: '5px 0',
                    borderRadius: '4px',
                  }}>curl http://localhost:8080/health</code>
                </li>
                <li>View frontend container logs:
                  <code style={{
                    display: 'block',
                    background: '#0a0e27',
                    padding: '8px',
                    margin: '5px 0',
                    borderRadius: '4px',
                  }}>docker compose logs frontend</code>
                </li>
                <li>Restart frontend container:
                  <code style={{
                    display: 'block',
                    background: '#0a0e27',
                    padding: '8px',
                    margin: '5px 0',
                    borderRadius: '4px',
                  }}>docker compose restart frontend</code>
                </li>
              </ol>
            </details>

            <div style={{
              display: 'flex',
              gap: '10px',
            }}>
              <button
                onClick={() => window.location.reload()}
                style={{
                  padding: '10px 20px',
                  backgroundColor: '#3498db',
                  color: '#fff',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: 'pointer',
                  fontSize: '14px',
                }}
              >
                Reload Page
              </button>
              <button
                onClick={() => this.setState({ hasError: false, error: null, errorInfo: null })}
                style={{
                  padding: '10px 20px',
                  backgroundColor: '#27ae60',
                  color: '#fff',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: 'pointer',
                  fontSize: '14px',
                }}
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}
