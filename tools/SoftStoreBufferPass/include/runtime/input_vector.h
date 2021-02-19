#ifndef __INPUT_VECTOR_H
#define __INPUT_VECTOR_H

class inputVector {
public:
  void feedInput(const uint32_t[], const std::size_t);
  uint32_t next();
  size_t size() { return inputSize; }

private:
#define kMaxInputSize 32
  std::size_t inputSize;
  uint32_t _vector[kMaxInputSize];
  int inputPos;
};

void inputVector::feedInput(const uint32_t vector[], const std::size_t size) {
  DEBUG_PRINT(std::cout << "Feeding input (" << size << "): { ");
  for (unsigned int i = 0; i < size; i++)
    DEBUG_PRINT(std::cout << vector[i] << ' ');
  DEBUG_PRINT(std::cout << "}" << std::endl);
  std::copy(vector, vector + size, _vector);
  inputSize = size;
}

uint32_t inputVector::next() {
  uint32_t pos = inputPos;
  inputPos = (inputPos + 1) % inputSize;
  return _vector[pos];
}

#endif // __INPUT_VECTOR_H
