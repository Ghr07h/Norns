import { Component, type ErrorInfo, type ReactNode } from "react";

export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };
  static getDerivedStateFromError(error: Error) { return { error }; }
  componentDidCatch(error: Error, info: ErrorInfo) { console.error("Well render error", error, info.componentStack); }
  render() {
    if (this.state.error) return <main className="fatal-error"><h1>Well</h1><p>{this.state.error.message}</p><button onClick={() => location.reload()}>Reload</button></main>;
    return this.props.children;
  }
}
