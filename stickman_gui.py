import tkinter as tk
import random

class StickmanApp:
    def __init__(self, root):
        self.root = root
        self.root.title("Random Stickman")
        self.canvas = tk.Canvas(root, width=400, height=400, bg="white")
        self.canvas.pack()
        btn = tk.Button(root, text="New Stickman", command=self.draw_stickman)
        btn.pack(pady=10)
        self.draw_stickman()

    def draw_stickman(self):
        self.canvas.delete("all")
        w, h = 400, 400
        x = random.randint(50, 350)
        y = random.randint(50, 350)
        self.canvas.create_oval(x-10, y-30, x+10, y-10, outline="black")
        self.canvas.create_line(x, y-10, x, y+40, fill="black", width=2)
        self.canvas.create_line(x-20, y+10, x+20, y+10, fill="black", width=2)
        self.canvas.create_line(x, y+40, x-20, y+80, fill="black", width=2)
        self.canvas.create_line(x, y+40, x+20, y+80, fill="black", width=2)

if __name__ == "__main__":
    root = tk.Tk()
    app = StickmanApp(root)
    root.mainloop()