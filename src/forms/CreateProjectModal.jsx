import { Modal, Form, Input } from "antd";

export default function CreateProjectModal({ open, onCancel, onCreate }) {
  const [form] = Form.useForm();

  return (
    <Modal
      title="Create Project"
      open={open}
      onCancel={onCancel}
      onOk={() => {
        form
          .validateFields()
          .then((values) => {
            onCreate(values);
            form.resetFields();
          })
          .catch(() => {});
      }}
      okText="Create"
    >
      <Form layout="vertical" form={form}>
        <Form.Item
          name="name"
          label="Project Name"
          rules={[{ required: true, message: "Enter a project name" }]}
        >
          <Input placeholder="e.g. Payments Platform" />
        </Form.Item>
      </Form>
    </Modal>
  );
}
