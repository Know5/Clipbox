import type { ErrorInfo, ReactNode } from "react";
import { Component } from "react";

export default class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(_error: Error, _info: ErrorInfo) {
    console.error("ClipBox interface failed to render");
  }
  render() {
    if (this.state.failed)
      return (
        <div className="app-error" role="alert">
          <strong>界面加载失败</strong>
          <p>后台仍会继续记录剪贴板。</p>
          <button className="btn-lite primary" onClick={() => this.setState({ failed: false })}>
            重新加载界面
          </button>
        </div>
      );
    return this.props.children;
  }
}
