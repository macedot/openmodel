from openai import OpenAI
client = OpenAI(
    api_key="b64a93db09d640c8b9ef56446c45b71d.uWddO5TFk8eeSzej",
    base_url="https://api.z.ai/api/paas/v4/"
)
completion = client.chat.completions.create(
    model="glm-4.7",
    messages=[
        {"role": "system", "content": "You are a helpful AI assistant."},
        {"role": "user", "content": "Hello, please introduce yourself."}
    ]
)
print(completion.choices[0].message.content)
