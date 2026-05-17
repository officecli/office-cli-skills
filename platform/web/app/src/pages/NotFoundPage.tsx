import { Button, Result } from 'antd'

export default function NotFoundPage() {
  return (
    <div className="space-y-8">
      <Result
        className="app-panel"
        status="404"
        title="Page not found"
        subTitle="The requested app workspace route does not exist."
        extra={<Button type="primary" href="/app/">Back to overview</Button>}
      />
    </div>
  )
}
